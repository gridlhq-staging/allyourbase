// Package server provides admin handlers for persisted search settings.
package server

import (
	"net/http"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/searchsettings"
)

type searchSettingsRequest struct {
	Attributes    []searchSettingAttribute     `json:"attributes"`
	CustomRanking []searchSettingCustomRanking `json:"customRanking,omitempty"`
}

type searchSettingAttribute struct {
	Column string `json:"column"`
	Weight string `json:"weight"`
}

type searchSettingCustomRanking struct {
	Column string `json:"column"`
	Order  string `json:"order"`
}

type searchSettingsResponse struct {
	Attributes    []searchSettingAttribute     `json:"attributes"`
	CustomRanking []searchSettingCustomRanking `json:"customRanking,omitempty"`
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
	rankings := make([]searchsettings.CustomRanking, 0, len(req.CustomRanking))
	for _, ranking := range req.CustomRanking {
		rankings = append(rankings, searchsettings.CustomRanking{
			Column: ranking.Column,
			Order:  searchsettings.RankingOrder(ranking.Order),
		})
	}
	return searchsettings.Settings{Attributes: attrs, CustomRanking: rankings}
}

func fromSearchSettings(settings searchsettings.Settings) searchSettingsResponse {
	attrs := make([]searchSettingAttribute, 0, len(settings.Attributes))
	for _, attr := range settings.Attributes {
		attrs = append(attrs, searchSettingAttribute{
			Column: attr.Column,
			Weight: string(attr.Weight),
		})
	}
	rankings := make([]searchSettingCustomRanking, 0, len(settings.CustomRanking))
	for _, ranking := range settings.CustomRanking {
		rankings = append(rankings, searchSettingCustomRanking{
			Column: ranking.Column,
			Order:  string(ranking.Order),
		})
	}
	return searchSettingsResponse{Attributes: attrs, CustomRanking: rankings}
}
