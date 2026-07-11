package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler serves the SSE realtime endpoint.
type Handler struct {
	hub         *Hub
	pool        *pgxpool.Pool // nil when RLS filtering unavailable
	authSvc     *auth.Service // nil when auth disabled
	schemaCache *schema.CacheHolder
	logger      *slog.Logger

	// CM is the optional ConnectionManager for cross-transport lifecycle governance.
	// When non-nil, SSE connections are registered/deregistered and subject to
	// per-user limits and drain behaviour.
	CM *ConnectionManager
}

const internalNotificationsTable = "_ayb_notifications"

// NewHandler creates a new realtime SSE handler.
// pool may be nil; when non-nil, events are filtered per-client via RLS.
func NewHandler(hub *Hub, pool *pgxpool.Pool, authSvc *auth.Service, schemaCache *schema.CacheHolder, logger *slog.Logger) *Handler {
	return &Handler{
		hub:         hub,
		pool:        pool,
		authSvc:     authSvc,
		schemaCache: schemaCache,
		logger:      logger,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httputil.WriteError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// OAuth SSE mode: no auth required, creates a one-time channel for OAuth result.
	if r.URL.Query().Get("oauth") == "true" {
		h.serveOAuthSSE(w, r, flusher)
		return
	}

	claims, ok := h.authenticateRealtimeRequest(w, r)
	if !ok {
		return
	}
	activeSchema := tenant.ActiveSchemaFromContext(r.Context())

	tablesParam := r.URL.Query().Get("tables")
	tables, ok := h.parseRealtimeTableSubscriptions(w, activeSchema, tablesParam)
	if !ok {
		return
	}

	filters, ok := h.parseRealtimeFilters(w, r.URL.Query().Get("filter"))
	if !ok {
		return
	}

	client, ctx, cleanup, ok := h.setupRealtimeSSEClient(w, r, claims, tables, filters)
	if !ok {
		return
	}
	defer cleanup()

	h.applySSEHeaders(w)

	// Send initial connected event.
	_, _ = fmt.Fprintf(w, "event: connected\ndata: {\"clientId\":%q}\n\n", client.ID)
	flusher.Flush()

	h.logger.Info("realtime client connected", "clientID", client.ID, "tables", tablesParam)
	h.streamRealtimeSSEEvents(w, flusher, ctx, claims, activeSchema, client)
}

// authenticateRealtimeRequest validates the bearer token or API key from the request, returning the authenticated claims or writing an error response if authentication fails.
func (h *Handler) authenticateRealtimeRequest(w http.ResponseWriter, r *http.Request) (*auth.Claims, bool) {
	if h.authSvc == nil {
		return nil, true
	}

	token, fromQuery := extractToken(r)
	if token == "" {
		httputil.WriteErrorWithDocURL(w, http.StatusUnauthorized, "authentication required",
			"https://allyourbase.io/guide/realtime")
		return nil, false
	}
	if fromQuery && auth.IsAPIKey(token) {
		httputil.WriteErrorWithDocURL(w, http.StatusUnauthorized, "API keys must be sent in the Authorization header",
			"https://allyourbase.io/guide/realtime")
		return nil, false
	}

	var (
		claims *auth.Claims
		err    error
	)
	// Support both JWT tokens and API keys (ayb_ prefix).
	if auth.IsAPIKey(token) {
		claims, err = h.authSvc.ValidateAPIKey(r.Context(), token)
	} else {
		claims, err = h.authSvc.ValidateToken(token)
	}
	if err != nil {
		httputil.WriteErrorWithDocURL(w, http.StatusUnauthorized, "invalid or expired token",
			"https://allyourbase.io/guide/realtime")
		return nil, false
	}

	return claims, true
}

// parseRealtimeTableSubscriptions splits the comma-separated tables parameter, validates each table name against the schema cache, and returns the subscription set.
func (h *Handler) parseRealtimeTableSubscriptions(w http.ResponseWriter, activeSchema, tablesParam string) (map[string]bool, bool) {
	if tablesParam == "" {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "tables parameter is required",
			"https://allyourbase.io/guide/realtime")
		return nil, false
	}

	tables := make(map[string]bool)
	sc := h.schemaCache.Get()
	for _, name := range strings.Split(tablesParam, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if sc != nil && !realtimeTableExistsInActiveSchema(sc, activeSchema, name) && name != internalNotificationsTable {
			httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "unknown table: "+name,
				"https://allyourbase.io/guide/realtime")
			return nil, false
		}
		tables[name] = true
	}
	if len(tables) == 0 {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "at least one valid table is required",
			"https://allyourbase.io/guide/realtime")
		return nil, false
	}

	return tables, true
}

func (h *Handler) parseRealtimeFilters(w http.ResponseWriter, filterParam string) (Filters, bool) {
	filters, err := ParseFilters(filterParam)
	if err != nil {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "invalid filter: "+err.Error(),
			"https://allyourbase.io/guide/realtime")
		return nil, false
	}
	return filters, true
}

// setupRealtimeSSEClient subscribes a new client to the realtime hub with the requested table filters, registers it with the connection manager if configured, and returns the client with a cleanup function.
func (h *Handler) setupRealtimeSSEClient(w http.ResponseWriter, r *http.Request, claims *auth.Claims, tables map[string]bool, filters Filters) (*Client, context.Context, func(), bool) {
	// Attach the request tenant at subscribe time so the client starts tenant-
	// scoped before any event can be delivered — no unregister/re-register churn.
	client := h.hub.SubscribeWithFilter(tables, filters, tenant.TenantFromContext(r.Context()))
	ctx, cancel := context.WithCancel(r.Context())

	cleanup := func() {
		cancel()
		h.hub.Unsubscribe(client.ID)
	}

	if h.CM == nil {
		return client, ctx, cleanup, true
	}

	meta := ConnectionMeta{
		ClientID:  client.ID,
		UserID:    UserKey(claims),
		Transport: "sse",
		CloseFunc: cancel,
		// SSE connections always subscribe to at least one table at connect time;
		// they are never idle-eligible.
		HasSubscriptions: func() bool { return true },
	}
	if err := h.CM.Register(meta); err != nil {
		cleanup()
		if errors.Is(err, ErrDraining) {
			httputil.WriteError(w, http.StatusServiceUnavailable, "server is shutting down")
		} else {
			httputil.WriteError(w, http.StatusTooManyRequests, "connection limit exceeded")
		}
		return nil, nil, nil, false
	}

	withDeregister := func() {
		h.CM.Deregister(client.ID)
		cleanup()
	}
	return client, ctx, withDeregister, true
}

func (h *Handler) applySSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
}

func (h *Handler) streamRealtimeSSEEvents(w http.ResponseWriter, flusher http.Flusher, ctx context.Context, claims *auth.Claims, activeSchema string, client *Client) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, open := <-client.Events():
			if !open {
				return
			}
			if !h.canSeeRecord(ctx, claims, activeSchema, event) {
				continue
			}
			if !shouldDeliverEvent(event, client.Filters()) {
				continue
			}
			// OldRecord is internal dispatch context for filter evaluation and
			// should not be exposed in transport payloads.
			data, err := json.Marshal(sanitizeEventForClient(event))
			if err != nil {
				h.logger.Error("failed to marshal event", "error", err, "clientID", client.ID)
				continue
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (h *Handler) serveOAuthSSE(w http.ResponseWriter, r *http.Request, flusher http.Flusher) {
	client := h.hub.SubscribeOAuth()
	defer h.hub.Unsubscribe(client.ID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send clientId — the SDK uses this as the OAuth state parameter.
	_, _ = fmt.Fprintf(w, "event: connected\ndata: {\"clientId\":%q}\n\n", client.ID)
	flusher.Flush()

	h.logger.Info("oauth SSE client connected", "clientID", client.ID)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case oauthEvent, open := <-client.OAuthEvents():
			if !open {
				return
			}
			data, err := json.Marshal(oauthEvent)
			if err != nil {
				h.logger.Error("failed to marshal oauth event", "error", err, "clientID", client.ID)
				continue
			}
			_, _ = fmt.Fprintf(w, "event: oauth\ndata: %s\n\n", data)
			flusher.Flush()
			return // OAuth flow is one-shot; close after delivering the result.
		}
	}
}

func (h *Handler) canSeeRecord(ctx context.Context, claims *auth.Claims, activeSchema string, event *Event) bool {
	return CanSeeRecord(ctx, h.pool, h.schemaCache, h.logger, claims, activeSchema, event)
}

// CanSeeRecord checks whether the authenticated user can see the event's record
// via an RLS-scoped SELECT. This per-event SELECT is evaluated by Postgres
// under the ayb_authenticated role, so full RLS policy logic applies, including
// join/EXISTS-based policies on related tables.
// Returns true when:
//   - no pool is available (RLS filtering disabled)
//   - no claims (unauthenticated client, no RLS applies)
//   - schema metadata or primary-key values are missing
//   - delete events lack OldRecord or SELECT-applicable RLS policies
//   - the RLS-scoped SELECT finds the row
func CanSeeRecord(ctx context.Context, pool *pgxpool.Pool, schemaCache *schema.CacheHolder, logger *slog.Logger, claims *auth.Claims, activeSchema string, event *Event) bool {
	if pool == nil || claims == nil {
		return true
	}

	sc := schemaCache.Get()
	if sc == nil {
		return true
	}
	tbl := sc.TableByNameInSchema(activeSchema, event.Table)
	if tbl != nil && activeSchema != "" && activeSchema != "public" && tbl.Schema != activeSchema && event.Table != internalNotificationsTable {
		return false
	}
	if tbl == nil || len(tbl.PrimaryKey) == 0 {
		if event.Table != internalNotificationsTable {
			return activeSchema == "" || activeSchema == "public"
		}

		id, ok := event.Record["id"]
		if !ok {
			return true
		}
		return runVisibilityCheck(ctx, pool, logger, claims, `SELECT 1 FROM public."_ayb_notifications" WHERE id = $1`, []any{id})
	}

	if event.Action == "delete" {
		return canSeeDeletedRecord(ctx, pool, logger, tbl, event.OldRecord, claims)
	}

	query, args := buildVisibilityCheck(tbl, event.Record)
	if query == "" {
		return activeSchema == "" || activeSchema == "public"
	}

	return runVisibilityCheck(ctx, pool, logger, claims, query, args)
}

func realtimeTableExistsInActiveSchema(sc *schema.SchemaCache, activeSchema, name string) bool {
	tbl := sc.TableByNameInSchema(activeSchema, name)
	if tbl == nil {
		return false
	}
	return activeSchema == "" || activeSchema == "public" || tbl.Schema == activeSchema
}

// canSeeDeletedRecord applies the delete-visibility truth table. It fails open
// for nil pool, nil claims, nil schema cache, missing table/PK metadata except
// _ayb_notifications, nil OldRecord, missing OldRecord PK values, and tables
// without RLS SELECT/ALL policies. Otherwise it evaluates OldRecord against the
// table's SELECT-applicable UsingExpr policies under the request user's RLS
// context and delivers only when the deleted row would have been visible.
func canSeeDeletedRecord(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, tbl *schema.Table, oldRecord map[string]any, claims *auth.Claims) bool {
	if oldRecord == nil || !recordHasPrimaryKeyValues(tbl, oldRecord) {
		return true
	}

	predicate, enforce := deleteVisibilityPredicate(tbl)
	if !enforce {
		return true
	}

	query, args := buildDeletedVisibilityCheck(tbl, predicate, oldRecord)
	return runVisibilityCheck(ctx, pool, logger, claims, query, args)
}

func runVisibilityCheck(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, claims *auth.Claims, query string, args []any) bool {
	tx, err := pool.Begin(ctx)
	if err != nil {
		logger.Error("rls filter: begin tx", "error", err)
		return false // fail closed
	}
	defer tx.Rollback(ctx)

	if err := auth.SetRLSContext(ctx, tx, claims); err != nil {
		logger.Error("rls filter: set rls context", "error", err)
		return false
	}

	var one int
	err = tx.QueryRow(ctx, query, args...).Scan(&one)
	return err == nil
}

func deleteVisibilityPredicate(tbl *schema.Table) (string, bool) {
	if !tbl.RLSEnabled {
		return "", false
	}
	var permissive []string
	var restrictive []string
	for _, policy := range tbl.RLSPolicies {
		if policy == nil {
			continue
		}
		command := strings.ToUpper(strings.TrimSpace(policy.Command))
		if command != "ALL" && command != "SELECT" {
			continue
		}
		expr := strings.TrimSpace(policy.UsingExpr)
		if expr == "" {
			expr = "TRUE"
		}
		if policy.Permissive {
			permissive = append(permissive, expr)
		} else {
			restrictive = append(restrictive, expr)
		}
	}
	if len(permissive) == 0 && len(restrictive) == 0 {
		return "", false
	}
	if len(permissive) == 0 {
		return "FALSE", true
	}
	clauses := append([]string{joinPolicyPredicates(permissive, " OR ")}, restrictive...)
	return joinPolicyPredicates(clauses, " AND "), true
}

func joinPolicyPredicates(predicates []string, sep string) string {
	wrapped := make([]string, len(predicates))
	for i, predicate := range predicates {
		wrapped[i] = "(" + predicate + ")"
	}
	return strings.Join(wrapped, sep)
}

func recordHasPrimaryKeyValues(tbl *schema.Table, record map[string]any) bool {
	for _, pk := range tbl.PrimaryKey {
		value, ok := record[pk]
		if !ok || value == nil {
			return false
		}
	}
	return true
}

func buildDeletedVisibilityCheck(tbl *schema.Table, predicate string, record map[string]any) (string, []any) {
	columns := make([]string, 0, len(record))
	for column := range record {
		columns = append(columns, column)
	}
	sort.Strings(columns)

	args := make([]any, 0, len(columns))
	placeholders := make([]string, len(columns))
	quotedColumns := make([]string, len(columns))
	for i, column := range columns {
		placeholders[i] = deletedRecordPlaceholder(tbl, column, i+1)
		quotedColumns[i] = sqlutil.QuoteIdent(column)
		args = append(args, record[column])
	}
	query := fmt.Sprintf("SELECT 1 FROM (VALUES (%s)) AS %s (%s) WHERE %s",
		strings.Join(placeholders, ", "),
		sqlutil.QuoteIdent(tbl.Name),
		strings.Join(quotedColumns, ", "),
		predicate)
	return query, args
}

// buildVisibilityCheck builds a SELECT 1 query scoped to a row's PK.
// Returns ("", nil) if the record is missing any PK column value.
func buildVisibilityCheck(tbl *schema.Table, record map[string]any) (string, []any) {
	args := make([]any, 0, len(tbl.PrimaryKey))
	var sb strings.Builder
	sb.WriteString("SELECT 1 FROM ")
	sb.WriteString(sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name))
	sb.WriteString(" WHERE ")

	for i, pk := range tbl.PrimaryKey {
		v, ok := record[pk]
		if !ok {
			return "", nil
		}
		if i > 0 {
			sb.WriteString(" AND ")
		}
		sb.WriteString(sqlutil.QuoteIdent(pk))
		sb.WriteString(" = $")
		sb.WriteString(strconv.Itoa(i + 1))
		args = append(args, v)
	}
	return sb.String(), args
}
