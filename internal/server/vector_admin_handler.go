package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/vector"
)

// vectorIndexRequest is the request body for POST /admin/vector/indexes.
type vectorIndexRequest struct {
	Schema    string `json:"schema"`
	Table     string `json:"table"`
	Column    string `json:"column"`
	Method    string `json:"method"`
	Metric    string `json:"metric"`
	IndexName string `json:"index_name"`
	Lists     int    `json:"lists"`
}

// vectorIndexInfo is a single index entry in the list response.
type vectorIndexInfo struct {
	Name       string `json:"name"`
	Schema     string `json:"schema"`
	Table      string `json:"table"`
	Method     string `json:"method"`
	Definition string `json:"definition"`
}

// resolveVectorIndexTable validates that req's table and column exist and that
// the column is a vector column, returning the table's actual schema. On a
// validation failure it writes a 400 response and returns ok=false.
//
// The schema cache is eventually consistent (the watcher reloads it on a
// debounced DDL notification), so a client that creates a table and
// immediately indexes it can race the reload. On a cache miss this forces a
// synchronous reload before concluding the table is absent — otherwise the
// request gets a spurious "table not found" 400.
func (s *Server) resolveVectorIndexTable(w http.ResponseWriter, r *http.Request, req *vectorIndexRequest) (schemaName string, ok bool) {
	sc := s.schema.Get()
	if (sc == nil || sc.TableByName(req.Table) == nil) && s.pool != nil {
		if err := s.schema.ReloadWait(r.Context()); err != nil {
			s.logger.Warn("vector index create: synchronous schema reload failed", "error", err)
		}
		sc = s.schema.Get()
	}
	// Without a cache there is nothing to validate against; keep the
	// caller-supplied schema and let the CREATE INDEX DDL be authoritative.
	if sc == nil {
		return req.Schema, true
	}

	tbl := sc.TableByName(req.Table)
	if tbl == nil {
		httputil.WriteError(w, http.StatusBadRequest, fmt.Sprintf("table %q not found", req.Table))
		return "", false
	}
	col := tbl.ColumnByName(req.Column)
	if col == nil {
		httputil.WriteError(w, http.StatusBadRequest, fmt.Sprintf("column %q not found in table %q", req.Column, req.Table))
		return "", false
	}
	if !col.IsVector {
		httputil.WriteError(w, http.StatusBadRequest, fmt.Sprintf("column %q is not a vector column (type: %s)", req.Column, col.TypeName))
		return "", false
	}
	return tbl.Schema, true
}

// handleAdminVectorIndexCreate handles POST /admin/vector/indexes.
func (s *Server) handleAdminVectorIndexCreate(w http.ResponseWriter, r *http.Request) {
	var req vectorIndexRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	// Validate required fields.
	var missing []string
	if req.Table == "" {
		missing = append(missing, "table")
	}
	if req.Column == "" {
		missing = append(missing, "column")
	}
	if req.Method == "" {
		missing = append(missing, "method")
	}
	if req.Metric == "" {
		missing = append(missing, "metric")
	}
	if len(missing) > 0 {
		httputil.WriteError(w, http.StatusBadRequest, "missing required fields: "+strings.Join(missing, ", "))
		return
	}

	// Default schema to public.
	if req.Schema == "" {
		req.Schema = "public"
	}

	// Validate the table/column and resolve the table's actual schema.
	resolvedSchema, ok := s.resolveVectorIndexTable(w, r, &req)
	if !ok {
		return
	}
	req.Schema = resolvedSchema

	// Auto-generate index name if not provided.
	if req.IndexName == "" {
		req.IndexName = fmt.Sprintf("idx_%s_%s_%s", req.Table, req.Column, req.Method)
	}

	// Build the DDL.
	ddl, err := vector.BuildCreateIndexSQL(vector.IndexParams{
		Schema:    req.Schema,
		Table:     req.Table,
		Column:    req.Column,
		Method:    req.Method,
		Metric:    req.Metric,
		IndexName: req.IndexName,
		Lists:     req.Lists,
	})
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Execute DDL using the pool directly — CREATE INDEX CONCURRENTLY
	// cannot run inside a transaction.
	if s.pool == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "database pool not available")
		return
	}

	if _, err := s.pool.Exec(r.Context(), ddl); err != nil {
		errMsg := err.Error()
		// Detect concurrent index builds.
		if strings.Contains(errMsg, "already building") || strings.Contains(errMsg, "CONCURRENTLY") {
			httputil.WriteError(w, http.StatusConflict, "another index build is already in progress")
			return
		}
		s.logger.Error("vector index create error", "error", err, "ddl", ddl)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to create index: "+errMsg)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]any{
		"index_name": req.IndexName,
		"method":     req.Method,
		"metric":     req.Metric,
		"table":      req.Table,
		"column":     req.Column,
	})
}

// vectorIndexMethods are the pgvector index methods we report on.
var vectorIndexMethods = map[string]bool{
	"hnsw":    true,
	"ivfflat": true,
}

// handleAdminVectorIndexList handles GET /admin/vector/indexes.
func (s *Server) handleAdminVectorIndexList(w http.ResponseWriter, r *http.Request) {
	// Use schema cache to list vector indexes — avoids requiring a pool.
	sc := s.schema.Get()
	if sc == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"indexes": []vectorIndexInfo{}})
		return
	}

	var indexes []vectorIndexInfo
	for _, tbl := range sc.Tables {
		for _, idx := range tbl.Indexes {
			if vectorIndexMethods[idx.Method] {
				indexes = append(indexes, vectorIndexInfo{
					Name:       idx.Name,
					Schema:     tbl.Schema,
					Table:      tbl.Name,
					Method:     idx.Method,
					Definition: idx.Definition,
				})
			}
		}
	}

	if indexes == nil {
		indexes = []vectorIndexInfo{}
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{"indexes": indexes})
}
