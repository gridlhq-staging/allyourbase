// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/jun04_pm_1_search_synonyms/allyourbase_dev/internal/server/search_synonyms_handler.go.
package server

import (
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/searchsynonyms"
	"github.com/go-chi/chi/v5"
)

type searchSynonymsRequest struct {
	Groups []searchSynonymGroup `json:"groups"`
}

type searchSynonymGroup struct {
	Terms []string `json:"terms"`
}

type searchSynonymsResponse struct {
	Groups []searchSynonymGroup `json:"groups"`
}

func (s *Server) handleSearchSynonymsGet(w http.ResponseWriter, r *http.Request) {
	tbl := resolveSearchSynonymCollection(w, r, s.schema)
	if tbl == nil {
		return
	}

	groups, err := searchsynonyms.NewStore(s.pool).LoadGroups(r.Context(), tbl.Schema, tbl.Name)
	if err != nil {
		s.logger.Error("load search synonyms error", "error", err, "schema", tbl.Schema, "table", tbl.Name)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load search synonyms")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, searchSynonymsResponse{Groups: fromSearchSynonymGroups(groups)})
}

func (s *Server) handleSearchSynonymsPut(w http.ResponseWriter, r *http.Request) {
	tbl := resolveSearchSynonymCollection(w, r, s.schema)
	if tbl == nil {
		return
	}

	var req searchSynonymsRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	groups, err := searchsynonyms.NormalizeGroups(toSearchSynonymGroups(req.Groups))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := searchsynonyms.NewStore(s.pool).ReplaceGroups(r.Context(), tbl.Schema, tbl.Name, groups); err != nil {
		s.logger.Error("replace search synonyms error", "error", err, "schema", tbl.Schema, "table", tbl.Name)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to save search synonyms")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, searchSynonymsResponse{Groups: fromSearchSynonymGroups(groups)})
}

func resolveSearchSynonymCollection(w http.ResponseWriter, r *http.Request, holder *schema.CacheHolder) *schema.Table {
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
	tbl := sc.TableByName(tableName)
	if tbl == nil {
		httputil.WriteError(w, http.StatusNotFound, "collection not found")
		return nil
	}
	return tbl
}

func toSearchSynonymGroups(groups []searchSynonymGroup) searchsynonyms.Groups {
	converted := make(searchsynonyms.Groups, 0, len(groups))
	for _, group := range groups {
		converted = append(converted, searchsynonyms.Group{Terms: group.Terms})
	}
	return converted
}

func fromSearchSynonymGroups(groups searchsynonyms.Groups) []searchSynonymGroup {
	converted := make([]searchSynonymGroup, 0, len(groups))
	for _, group := range groups {
		converted = append(converted, searchSynonymGroup{Terms: group.Terms})
	}
	return converted
}
