package server

import (
	"net/http"
	"time"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/jackc/pgx/v5"
)

type adminRequestLogAggregateBucket struct {
	Bucket    time.Time `json:"bucket"`
	Count     int       `json:"count"`
	Status2xx int       `json:"status_2xx"`
	Status3xx int       `json:"status_3xx"`
	Status4xx int       `json:"status_4xx"`
	Status5xx int       `json:"status_5xx"`
}

type adminRequestLogAggregateResponse struct {
	Items []adminRequestLogAggregateBucket `json:"items"`
	Count int                              `json:"count"`
}

func (s *Server) handleAdminRequestLogsAggregate(w http.ResponseWriter, r *http.Request) {
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
		httputil.WriteError(w, http.StatusInternalServerError, "failed to query request log aggregates")
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(r.Context())
		}
	}()

	sql, args := buildAdminRequestLogsAggregateQuery(filters)
	rows, err := tx.Query(r.Context(), sql, args...)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to query request log aggregates")
		return
	}

	items, scanErr := scanAdminRequestLogAggregateRows(rows)
	if scanErr != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to decode request log aggregate row")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "failed to query request log aggregates")
		return
	}
	committed = true

	httputil.WriteJSON(w, http.StatusOK, adminRequestLogAggregateResponse{
		Items: items,
		Count: len(items),
	})
}

func buildAdminRequestLogsAggregateQuery(filters adminRequestLogFilters) (string, []any) {
	whereSQL, args := buildAdminRequestLogsWhereClause(filters)
	return `SELECT date_trunc('minute', timestamp) AS bucket,
				COUNT(*),
				COUNT(*) FILTER (WHERE status_code >= 200 AND status_code <= 299),
				COUNT(*) FILTER (WHERE status_code >= 300 AND status_code <= 399),
				COUNT(*) FILTER (WHERE status_code >= 400 AND status_code <= 499),
				COUNT(*) FILTER (WHERE status_code >= 500 AND status_code <= 599)
			FROM _ayb_request_logs` + whereSQL + `
			GROUP BY bucket
			ORDER BY bucket ASC`, args
}

func scanAdminRequestLogAggregateRows(rows pgx.Rows) ([]adminRequestLogAggregateBucket, error) {
	defer rows.Close()

	items := make([]adminRequestLogAggregateBucket, 0)
	for rows.Next() {
		var item adminRequestLogAggregateBucket
		if err := rows.Scan(
			&item.Bucket,
			&item.Count,
			&item.Status2xx,
			&item.Status3xx,
			&item.Status4xx,
			&item.Status5xx,
		); err != nil {
			return nil, err
		}
		item.Bucket = item.Bucket.UTC()
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
