package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	adminRequestLogsStreamBatchLimit     = 100
	adminRequestLogsStreamPollIntervalMS = 250
)

var adminRequestLogsStreamPollInterval = time.Duration(adminRequestLogsStreamPollIntervalMS) * time.Millisecond

type adminRequestLogHighWaterMark struct {
	timestamp time.Time
	id        string
}

func (s *Server) handleAdminRequestLogsStream(w http.ResponseWriter, r *http.Request) {
	filters, badRequestMessage := parseAdminRequestLogFilters(r.URL.Query())
	if badRequestMessage != "" {
		httputil.WriteError(w, http.StatusBadRequest, badRequestMessage)
		return
	}
	if s.pool == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		httputil.WriteError(w, http.StatusInternalServerError, "streaming is not supported by this server")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	cursor, err := queryAdminRequestLogsHighWater(r.Context(), s.pool, filters)
	if err != nil {
		writeAdminRequestLogsStreamError(w, flusher, r.Context())
		return
	}
	if err := writeSSEJSON(w, flusher, "ready", map[string]any{
		"delivery":         "polling",
		"poll_interval_ms": adminRequestLogsStreamPollIntervalMS,
	}); err != nil {
		return
	}

	s.pollAdminRequestLogsStream(r.Context(), w, flusher, filters, cursor)
}

func (s *Server) pollAdminRequestLogsStream(
	ctx context.Context,
	w http.ResponseWriter,
	flusher http.Flusher,
	filters adminRequestLogFilters,
	cursor adminRequestLogHighWaterMark,
) {
	ticker := time.NewTicker(adminRequestLogsStreamPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			nextCursor, ok := s.emitAdminRequestLogsStreamBatch(ctx, w, flusher, filters, cursor)
			if !ok {
				return
			}
			cursor = nextCursor
		}
	}
}

func (s *Server) emitAdminRequestLogsStreamBatch(
	ctx context.Context,
	w http.ResponseWriter,
	flusher http.Flusher,
	filters adminRequestLogFilters,
	cursor adminRequestLogHighWaterMark,
) (adminRequestLogHighWaterMark, bool) {
	items, err := queryAdminRequestLogsAfter(ctx, s.pool, filters, cursor)
	if err != nil {
		writeAdminRequestLogsStreamError(w, flusher, ctx)
		return cursor, false
	}
	for _, item := range items {
		if ctx.Err() != nil {
			return cursor, false
		}
		if err := writeSSEJSON(w, flusher, "request_log", item); err != nil {
			return cursor, false
		}
		cursor = adminRequestLogHighWaterMark{timestamp: item.Timestamp, id: item.ID}
	}
	return cursor, true
}

func writeAdminRequestLogsStreamError(w http.ResponseWriter, flusher http.Flusher, ctx context.Context) {
	if isCanceledStreamRequest(ctx) {
		return
	}
	_ = writeSSEJSON(w, flusher, "error", map[string]any{"message": "failed to query request logs"})
}

func queryAdminRequestLogsHighWater(
	ctx context.Context,
	pool *pgxpool.Pool,
	filters adminRequestLogFilters,
) (adminRequestLogHighWaterMark, error) {
	sql, args := buildAdminRequestLogsHighWaterQuery(filters)
	cursor := adminRequestLogHighWaterMark{id: uuid.Nil.String()}
	err := pool.QueryRow(ctx, sql, args...).Scan(&cursor.timestamp, &cursor.id)
	if err == nil {
		return cursor, nil
	}
	if err == pgx.ErrNoRows {
		return cursor, nil
	}
	return adminRequestLogHighWaterMark{}, err
}

func queryAdminRequestLogsAfter(
	ctx context.Context,
	pool *pgxpool.Pool,
	filters adminRequestLogFilters,
	cursor adminRequestLogHighWaterMark,
) ([]adminRequestLogEntry, error) {
	sql, args := buildAdminRequestLogsLiveQuery(
		filters,
		cursor.timestamp,
		cursor.id,
		adminRequestLogsStreamBatchLimit,
	)
	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return scanAdminRequestLogRows(rows, adminRequestLogsStreamBatchLimit)
}

func scanAdminRequestLogRows(rows pgx.Rows, capacity int) ([]adminRequestLogEntry, error) {
	defer rows.Close()

	items := make([]adminRequestLogEntry, 0, capacity)
	for rows.Next() {
		var item adminRequestLogEntry
		if err := rows.Scan(
			&item.ID, &item.Timestamp, &item.Method, &item.Path,
			&item.StatusCode, &item.DurationMS,
			&item.UserID, &item.APIKeyID,
			&item.RequestSize, &item.ResponseSize,
			&item.IPAddress, &item.RequestID,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func buildAdminRequestLogsHighWaterQuery(filters adminRequestLogFilters) (string, []any) {
	whereSQL, args := buildAdminRequestLogsWhereClause(filters)
	return `SELECT timestamp, id
			FROM _ayb_request_logs` + whereSQL + `
			ORDER BY timestamp DESC, id DESC LIMIT 1`, args
}

func buildAdminRequestLogsLiveQuery(
	filters adminRequestLogFilters,
	afterTimestamp time.Time,
	afterID string,
	limit int,
) (string, []any) {
	sql := `SELECT id, timestamp, method, path, status_code, duration_ms,
				user_id, api_key_id, request_size, response_size, host(ip_address), request_id
			FROM _ayb_request_logs`
	whereSQL, args := buildAdminRequestLogsWhereClause(filters)
	sql += whereSQL

	separator := "\nWHERE "
	if whereSQL != "" {
		separator = " AND "
	}
	argPos := len(args) + 1
	sql += fmt.Sprintf("%s(timestamp, id) > ($%d, $%d)", separator, argPos, argPos+1)
	args = append(args, afterTimestamp, afterID)
	argPos += 2
	sql += fmt.Sprintf("\nORDER BY timestamp ASC, id ASC LIMIT $%d", argPos)
	args = append(args, limit)
	return sql, args
}
