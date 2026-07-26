package server

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type adminRequestLogEntry struct {
	ID           string    `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	Method       string    `json:"method"`
	Path         string    `json:"path"`
	StatusCode   int       `json:"status_code"`
	DurationMS   int64     `json:"duration_ms"`
	UserID       *string   `json:"user_id,omitempty"`
	APIKeyID     *string   `json:"api_key_id,omitempty"`
	RequestSize  int64     `json:"request_size"`
	ResponseSize int64     `json:"response_size"`
	IPAddress    *string   `json:"ip_address,omitempty"`
	RequestID    *string   `json:"request_id,omitempty"`
}

type adminRequestLogListResponse struct {
	Items  []adminRequestLogEntry `json:"items"`
	Count  int                    `json:"count"`
	Limit  int                    `json:"limit"`
	Offset int                    `json:"offset"`
}

type adminRequestLogFilters struct {
	method          string
	tenantID        string
	tenantIDSet     bool
	path            string
	statusCode      int
	statusClassMin  int
	statusClassMax  int
	minDurationMS   *int64
	maxDurationMS   *int64
	limit           int
	offset          int
	fromTime        time.Time
	toTime          time.Time
	toDateOnly      bool
	cursorTimestamp time.Time
	cursorID        string
}

// handleAdminRequestLogs returns filtered and paginated request logs plus the exact total match count.
func (s *Server) handleAdminRequestLogs(w http.ResponseWriter, r *http.Request) {
	filters, badRequestMessage := parseAdminRequestLogFilters(r.URL.Query())
	if badRequestMessage != "" {
		httputil.WriteError(w, http.StatusBadRequest, badRequestMessage)
		return
	}

	if s.pool == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}

	tx, err := s.pool.BeginTx(r.Context(), adminRequestLogsReadTransactionOptions())
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to query request logs")
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(r.Context())
		}
	}()

	countSQL, countArgs := buildAdminRequestLogsCountQuery(filters)
	var count int
	if err := tx.QueryRow(r.Context(), countSQL, countArgs...).Scan(&count); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to count request logs")
		return
	}

	sql, args := buildAdminRequestLogsQuery(filters)
	rows, err := tx.Query(r.Context(), sql, args...)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to query request logs")
		return
	}

	items, scanErr := scanAdminRequestLogRows(rows, filters.limit)
	if scanErr != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to decode request log row")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to query request logs")
		return
	}
	committed = true

	httputil.WriteJSON(w, http.StatusOK, adminRequestLogListResponse{
		Items:  items,
		Count:  count,
		Limit:  filters.limit,
		Offset: filters.offset,
	})
}

func adminRequestLogsReadTransactionOptions() pgx.TxOptions {
	return pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	}
}

// parseAdminRequestLogFilters extracts request log query filters (method, path, status code, pagination, time range) from URL query parameters, returning a validation error message if any value is invalid.
func parseAdminRequestLogFilters(query url.Values) (adminRequestLogFilters, string) {
	_, tenantIDSet := query["tenant_id"]
	filters := adminRequestLogFilters{
		method:      strings.ToUpper(strings.TrimSpace(query.Get("method"))),
		tenantID:    strings.TrimSpace(query.Get("tenant_id")),
		tenantIDSet: tenantIDSet,
		path:        strings.TrimSpace(query.Get("path")),
		limit:       defaultAdminListLimit,
	}

	if rawStatus := strings.TrimSpace(query.Get("status")); rawStatus != "" {
		parsed, err := strconv.Atoi(rawStatus)
		if err != nil || parsed < 100 || parsed > 599 {
			return filters, "invalid status; must be an integer 100–599"
		}
		filters.statusCode = parsed
	}

	switch rawStatusClass := strings.TrimSpace(query.Get("status_class")); rawStatusClass {
	case "":
	case "2xx":
		filters.statusClassMin, filters.statusClassMax = 200, 299
	case "3xx":
		filters.statusClassMin, filters.statusClassMax = 300, 399
	case "4xx":
		filters.statusClassMin, filters.statusClassMax = 400, 499
	case "5xx":
		filters.statusClassMin, filters.statusClassMax = 500, 599
	default:
		return filters, "invalid status_class; must be one of 2xx, 3xx, 4xx, 5xx"
	}

	var errMsg string
	filters.minDurationMS, errMsg = parseAdminRequestLogDuration(query, "min_duration_ms")
	if errMsg != "" {
		return filters, errMsg
	}
	filters.maxDurationMS, errMsg = parseAdminRequestLogDuration(query, "max_duration_ms")
	if errMsg != "" {
		return filters, errMsg
	}
	if filters.minDurationMS != nil && filters.maxDurationMS != nil &&
		*filters.minDurationMS > *filters.maxDurationMS {
		return filters, "min_duration_ms must be less than or equal to max_duration_ms"
	}

	limit, offset, errMsg := parseAdminListPagination(query)
	if errMsg != "" {
		return filters, errMsg
	}
	filters.limit = limit
	filters.offset = offset

	timeRange, errMsg := parseAdminTimeRange(query)
	if errMsg != "" {
		return filters, errMsg
	}
	filters.fromTime = timeRange.fromTime
	filters.toTime = timeRange.toTime
	filters.toDateOnly = timeRange.toDateOnly

	cursorTimestamp, cursorID, errMsg := parseAdminRequestLogCursor(query)
	if errMsg != "" {
		return filters, errMsg
	}
	filters.cursorTimestamp = cursorTimestamp
	filters.cursorID = cursorID

	return filters, ""
}

func parseAdminRequestLogCursor(query url.Values) (time.Time, string, string) {
	rawTimestamp := strings.TrimSpace(query.Get("cursor_timestamp"))
	rawID := strings.TrimSpace(query.Get("cursor_id"))
	if (rawTimestamp == "") != (rawID == "") {
		return time.Time{}, "", "cursor_timestamp and cursor_id must be provided together"
	}
	if rawTimestamp == "" {
		return time.Time{}, "", ""
	}

	cursorTimestamp, err := time.Parse(time.RFC3339, rawTimestamp)
	if err != nil {
		return time.Time{}, "", "invalid cursor_timestamp; expected RFC3339"
	}
	cursorID, err := uuid.Parse(rawID)
	if err != nil {
		return time.Time{}, "", "invalid cursor_id; expected UUID"
	}
	return cursorTimestamp, cursorID.String(), ""
}

func parseAdminRequestLogDuration(query url.Values, name string) (*int64, string) {
	rawDuration := strings.TrimSpace(query.Get(name))
	if rawDuration == "" {
		return nil, ""
	}
	duration, err := strconv.ParseInt(rawDuration, 10, 64)
	if err != nil || duration < 0 {
		return nil, fmt.Sprintf("invalid %s; must be a non-negative integer", name)
	}
	return &duration, ""
}

// buildAdminRequestLogsQuery constructs the ordered, paginated request-log query.
func buildAdminRequestLogsQuery(filters adminRequestLogFilters) (string, []any) {
	sql := `SELECT id, timestamp, method, path, status_code, duration_ms,
				user_id, api_key_id, request_size, response_size, host(ip_address), request_id
			FROM _ayb_request_logs`

	whereSQL, args := buildAdminRequestLogsWhereClause(filters)
	sql += whereSQL
	if !filters.cursorTimestamp.IsZero() {
		separator := "\nWHERE "
		if whereSQL != "" {
			separator = " AND "
		}
		argPos := len(args) + 1
		sql += fmt.Sprintf("%s(timestamp, id) < ($%d, $%d)", separator, argPos, argPos+1)
		args = append(args, filters.cursorTimestamp, filters.cursorID)
	}
	argPos := len(args) + 1
	sql += fmt.Sprintf("\nORDER BY timestamp DESC, id DESC LIMIT $%d OFFSET $%d", argPos, argPos+1)
	args = append(args, filters.limit, filters.offset)
	return sql, args
}

func buildAdminRequestLogsCountQuery(filters adminRequestLogFilters) (string, []any) {
	whereSQL, args := buildAdminRequestLogsWhereClause(filters)
	return "SELECT COUNT(*) FROM _ayb_request_logs" + whereSQL, args
}

// Both page and count queries use this seam so their filter semantics cannot drift.
func buildAdminRequestLogsWhereClause(filters adminRequestLogFilters) (string, []any) {
	var whereClauses []string
	var args []any
	argPos := 1

	if filters.method != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("method = $%d", argPos))
		args = append(args, filters.method)
		argPos++
	}
	if filters.tenantIDSet {
		whereClauses = append(whereClauses, fmt.Sprintf("tenant_id = $%d", argPos))
		args = append(args, filters.tenantID)
		argPos++
	}
	if filters.path != "" {
		clause, likePath, nextArgPos := buildPathLikeClause(filters.path, argPos)
		whereClauses = append(whereClauses, clause)
		args = append(args, likePath)
		argPos = nextArgPos
	}
	if filters.statusCode != 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("status_code = $%d", argPos))
		args = append(args, filters.statusCode)
		argPos++
	}
	if filters.statusClassMin != 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("status_code >= $%d", argPos))
		args = append(args, filters.statusClassMin)
		argPos++
	}
	if filters.statusClassMax != 0 {
		whereClauses = append(whereClauses, fmt.Sprintf("status_code <= $%d", argPos))
		args = append(args, filters.statusClassMax)
		argPos++
	}
	if filters.minDurationMS != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("duration_ms >= $%d", argPos))
		args = append(args, *filters.minDurationMS)
		argPos++
	}
	if filters.maxDurationMS != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("duration_ms <= $%d", argPos))
		args = append(args, *filters.maxDurationMS)
		argPos++
	}
	if !filters.fromTime.IsZero() {
		whereClauses = append(whereClauses, fmt.Sprintf("timestamp >= $%d", argPos))
		args = append(args, filters.fromTime)
		argPos++
	}
	if !filters.toTime.IsZero() {
		if filters.toDateOnly {
			whereClauses = append(whereClauses, fmt.Sprintf("timestamp < $%d", argPos))
			args = append(args, filters.toTime.Add(24*time.Hour))
		} else {
			whereClauses = append(whereClauses, fmt.Sprintf("timestamp <= $%d", argPos))
			args = append(args, filters.toTime)
		}
		argPos++
	}

	if len(whereClauses) > 0 {
		return "\nWHERE " + strings.Join(whereClauses, " AND "), args
	}
	return "", args
}

func buildPathLikeClause(path string, argPos int) (string, string, int) {
	escaped := strings.NewReplacer(
		"\\", "\\\\",
		"%", "\\%",
		"_", "\\_",
	).Replace(path)
	// Users provide * wildcards in query params; convert to SQL LIKE.
	escaped = strings.ReplaceAll(escaped, "*", "%")
	return fmt.Sprintf("path LIKE $%d ESCAPE '\\'", argPos), escaped, argPos + 1
}
