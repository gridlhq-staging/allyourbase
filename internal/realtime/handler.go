package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/schema"
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

// ServeHTTP serves filtered table events or one-shot OAuth results over SSE.
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
			httputil.DocURL("/guide/realtime"))
		return nil, false
	}
	if fromQuery && auth.IsAPIKey(token) {
		httputil.WriteErrorWithDocURL(w, http.StatusUnauthorized, "API keys must be sent in the Authorization header",
			httputil.DocURL("/guide/realtime"))
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
			httputil.DocURL("/guide/realtime"))
		return nil, false
	}

	return claims, true
}

// parseRealtimeTableSubscriptions splits the comma-separated tables parameter, validates each table name against the schema cache, and returns the subscription set.
func (h *Handler) parseRealtimeTableSubscriptions(w http.ResponseWriter, activeSchema, tablesParam string) (map[string]bool, bool) {
	if tablesParam == "" {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "tables parameter is required",
			httputil.DocURL("/guide/realtime"))
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
				httputil.DocURL("/guide/realtime"))
			return nil, false
		}
		tables[name] = true
	}
	if len(tables) == 0 {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "at least one valid table is required",
			httputil.DocURL("/guide/realtime"))
		return nil, false
	}

	return tables, true
}

func (h *Handler) parseRealtimeFilters(w http.ResponseWriter, filterParam string) (Filters, bool) {
	filters, err := ParseFilters(filterParam)
	if err != nil {
		httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "invalid filter: "+err.Error(),
			httputil.DocURL("/guide/realtime"))
		return nil, false
	}
	return filters, true
}

// setupRealtimeSSEClient subscribes a new client to the realtime hub with the requested table filters, registers it with the connection manager if configured, and returns the client with a cleanup function.
func (h *Handler) setupRealtimeSSEClient(w http.ResponseWriter, r *http.Request, claims *auth.Claims, tables map[string]bool, filters Filters) (*Client, context.Context, func(), bool) {
	// Attach the request tenant at subscribe time so the client starts tenant-
	// scoped before any event can be delivered — no unregister/re-register churn.
	client := h.hub.SubscribeWithFilter(tables, filters, h.realtimeTenantScope(r, claims))
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

func (h *Handler) realtimeTenantScope(r *http.Request, claims *auth.Claims) string {
	if claims != nil && h.pool != nil && h.schemaCache != nil {
		return RLSFilteredTenantScope
	}
	return tenant.TenantFromContext(r.Context())
}

func (h *Handler) applySSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
}

// streamRealtimeSSEEvents sends visible, subscription-matching table events until the client disconnects.
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

// serveOAuthSSE exposes a one-shot OAuth result stream whose client ID identifies the flow.
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
//
// Because RLS-filtered subscribers bypass the Hub tenant gate (see
// RLSFilteredTenantScope / tenantMatches), this function is the single source of
// truth for per-record visibility, including tenant isolation. A candidate
// tagged for a different tenant is dropped unless BOTH the table has the
// metadata and RLS SELECT policy needed to build a per-record visibility query
// AND the candidate's own record carries the PK values that query binds —
// either half missing would otherwise fail open and leak it across tenants.
//
// Authenticated clients need a database pool to prove row visibility; without
// one, realtime visibility fails closed. Unauthenticated clients keep the
// no-RLS path because there are no user claims to scope.
//
// Returns true when:
//   - no claims are present (unauthenticated client, no RLS applies)
//   - schema metadata or primary-key values are missing AND the candidate is not
//     tagged for a foreign tenant
//   - delete events lack SELECT-applicable RLS policies AND the candidate is not
//     tagged for a foreign tenant
//   - the RLS-scoped SELECT finds the row
func CanSeeRecord(ctx context.Context, pool *pgxpool.Pool, schemaCache *schema.CacheHolder, logger *slog.Logger, claims *auth.Claims, activeSchema string, event *Event) bool {
	if claims == nil {
		return true
	}
	if pool == nil {
		return false
	}

	// A candidate tagged for another tenant is the only case the tenant floor
	// below can never wave through, so it is decided ahead of every bypass.
	foreignTenant := event.TenantID != "" && event.TenantID != claims.TenantID

	sc := schemaCache.Get()
	if sc == nil {
		// The cache is unloaded (server startup gates on CacheHolder.Ready(), so
		// this is reachable only for embedders that serve before the first
		// introspection). No table metadata means no proof path, so a
		// foreign-tenant candidate must be dropped rather than fall through.
		return !foreignTenant
	}
	tbl := sc.TableByNameInSchema(activeSchema, event.Table)
	if tbl != nil && activeSchema != "" && activeSchema != "public" && tbl.Schema != activeSchema && event.Table != internalNotificationsTable {
		return false
	}

	// Tenant-isolation floor. The per-record RLS SELECT below can only NARROW
	// visibility within the subscriber's own tenant; it cannot be trusted to
	// authorize a row tagged for a different tenant unless the table can build
	// a per-record visibility query from PK metadata and SELECT-applicable RLS
	// policies. Drop the foreign-tenant candidate instead of failing open. Empty
	// event.TenantID is the _ayb_notifications / wildcard case and is handled by
	// the checks below.
	if foreignTenant && !canProveRecordVisibility(tbl) {
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
		// canProveRecordVisibility only proves the TABLE can be checked; the
		// deleted ROW still has to carry PK values or canSeeDeletedRecord fails
		// open. That fail-open is safe only inside the subscriber's own tenant,
		// so drop an unprovable foreign-tenant candidate here.
		if foreignTenant && !schema.RecordHasPrimaryKeyValues(tbl, event.OldRecord) {
			return false
		}
		return canSeeDeletedRecord(ctx, pool, logger, tbl, event.OldRecord, claims)
	}

	query, args := buildVisibilityCheck(tbl, event.Record)
	if query == "" {
		// The record carries no PK value, so no per-record RLS SELECT can prove
		// it visible. Publishers are not required to echo the table's PK (the
		// RPC X-Notify-Table path forwards whatever the function returned), so a
		// foreign-tenant candidate must not inherit the public-schema fail-open.
		return !foreignTenant && (activeSchema == "" || activeSchema == "public")
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
