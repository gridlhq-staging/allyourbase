// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/jun09_pm_4_search_relevance_weighting_and_custom_ranking/allyourbase_dev/internal/server/admin_collection_resolver.go.
package server

import (
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/go-chi/chi/v5"
)

func resolveAdminCollection(w http.ResponseWriter, r *http.Request, holder *schema.CacheHolder) *schema.Table {
	tableName := chi.URLParam(r, "table")
	if holder == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "schema cache not ready")
		return nil
	}
	sc := holder.Get()
	if sc == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "schema cache not ready")
		return nil
	}

	// Prefer public collections for backwards compatibility, but reject
	// ambiguous non-public matches instead of selecting an arbitrary map entry.
	if tbl := sc.Tables["public."+tableName]; tbl != nil {
		return tbl
	}
	var tbl *schema.Table
	for _, candidate := range sc.Tables {
		if candidate.Name != tableName {
			continue
		}
		if tbl != nil {
			httputil.WriteError(w, http.StatusConflict, "collection name is ambiguous across schemas")
			return nil
		}
		tbl = candidate
	}
	if tbl == nil {
		httputil.WriteError(w, http.StatusNotFound, "collection not found")
		return nil
	}
	return tbl
}
