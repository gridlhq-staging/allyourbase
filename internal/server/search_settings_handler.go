// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/jun09_pm_4_search_relevance_weighting_and_custom_ranking/allyourbase_dev/internal/server/search_settings_handler.go.
package server

import (
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/searchsettings"
)

type searchSettingsRequest struct {
	Attributes []searchSettingAttribute `json:"attributes"`
}

type searchSettingAttribute struct {
	Column string `json:"column"`
	Weight string `json:"weight"`
}

type searchSettingsResponse struct {
	Attributes []searchSettingAttribute `json:"attributes"`
}

func (s *Server) handleSearchSettingsGet(w http.ResponseWriter, r *http.Request) {
	tbl := resolveAdminCollection(w, r, s.schema)
	if tbl == nil {
		return
	}

	settings, err := searchsettings.NewStore(s.pool).Load(r.Context(), tbl.Schema, tbl.Name)
	if err != nil {
		s.logger.Error("load search settings error", "error", err, "schema", tbl.Schema, "table", tbl.Name)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load search settings")
		return
	}
	settings, err = searchsettings.ValidateForTable(tbl, settings)
	if err != nil {
		s.logger.Error("invalid search settings", "error", err, "schema", tbl.Schema, "table", tbl.Name)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load search settings")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, fromSearchSettings(settings))
}

func (s *Server) handleSearchSettingsPut(w http.ResponseWriter, r *http.Request) {
	tbl := resolveAdminCollection(w, r, s.schema)
	if tbl == nil {
		return
	}

	var req searchSettingsRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}
	settings, err := searchsettings.ValidateForTable(tbl, toSearchSettings(req))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := searchsettings.NewStore(s.pool).Save(r.Context(), tbl.Schema, tbl.Name, settings); err != nil {
		s.logger.Error("save search settings error", "error", err, "schema", tbl.Schema, "table", tbl.Name)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to save search settings")
		return
	}
	if err := s.schema.ReloadWait(r.Context()); err != nil {
		s.logger.Error("reload search settings indexes error", "error", err, "schema", tbl.Schema, "table", tbl.Name)
		httputil.WriteError(w, http.StatusInternalServerError, "saved search settings but failed to refresh search indexes")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, fromSearchSettings(settings))
}

func toSearchSettings(req searchSettingsRequest) searchsettings.Settings {
	attrs := make([]searchsettings.Attribute, 0, len(req.Attributes))
	for _, attr := range req.Attributes {
		attrs = append(attrs, searchsettings.Attribute{
			Column: attr.Column,
			Weight: searchsettings.Weight(attr.Weight),
		})
	}
	return searchsettings.Settings{Attributes: attrs}
}

func fromSearchSettings(settings searchsettings.Settings) searchSettingsResponse {
	attrs := make([]searchSettingAttribute, 0, len(settings.Attributes))
	for _, attr := range settings.Attributes {
		attrs = append(attrs, searchSettingAttribute{
			Column: attr.Column,
			Weight: string(attr.Weight),
		})
	}
	return searchSettingsResponse{Attributes: attrs}
}
