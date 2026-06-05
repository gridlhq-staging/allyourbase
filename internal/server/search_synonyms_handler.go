// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/jun04_pm_1_search_synonyms/allyourbase_dev/internal/server/search_synonyms_handler.go.
package server

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxSearchSynonymTermLength = 128
	maxSearchSynonymGroupTerms = 8
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

type normalizedSearchSynonymGroups []searchSynonymGroup

func (s *Server) handleSearchSynonymsGet(w http.ResponseWriter, r *http.Request) {
	tbl := resolveSearchSynonymCollection(w, r, s.schema)
	if tbl == nil {
		return
	}

	groups, err := loadSearchSynonymGroups(r.Context(), s.pool, tbl.Schema, tbl.Name)
	if err != nil {
		s.logger.Error("load search synonyms error", "error", err, "schema", tbl.Schema, "table", tbl.Name)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to load search synonyms")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, searchSynonymsResponse{Groups: groups})
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
	groups, err := normalizeSearchSynonymGroups(req.Groups)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := replaceSearchSynonymGroups(r.Context(), s.pool, tbl.Schema, tbl.Name, groups); err != nil {
		s.logger.Error("replace search synonyms error", "error", err, "schema", tbl.Schema, "table", tbl.Name)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to save search synonyms")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, searchSynonymsResponse{Groups: groups})
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

func normalizeSearchSynonymGroups(groups []searchSynonymGroup) (normalizedSearchSynonymGroups, error) {
	if len(groups) == 0 {
		return nil, fmt.Errorf("groups is required")
	}

	seen := make(map[string]struct{})
	normalized := make(normalizedSearchSynonymGroups, 0, len(groups))
	for _, group := range groups {
		terms, err := normalizeSearchSynonymTerms(group.Terms, seen)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, searchSynonymGroup{Terms: terms})
	}
	sortSearchSynonymGroups(normalized)
	return normalized, nil
}

func normalizeSearchSynonymTerms(terms []string, seen map[string]struct{}) ([]string, error) {
	normalized := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term == "" {
			continue
		}
		if len(term) > maxSearchSynonymTermLength {
			return nil, fmt.Errorf("synonym terms must be 128 characters or fewer")
		}
		if _, ok := seen[term]; ok {
			return nil, fmt.Errorf("duplicate synonym term: %s", term)
		}
		seen[term] = struct{}{}
		normalized = append(normalized, term)
	}
	if len(normalized) < 2 {
		return nil, fmt.Errorf("each synonym group must include at least two terms")
	}
	if len(normalized) > maxSearchSynonymGroupTerms {
		return nil, fmt.Errorf("synonym groups may include at most 8 terms")
	}
	sort.Strings(normalized)
	return normalized, nil
}

func sortSearchSynonymGroups(groups []searchSynonymGroup) {
	sort.Slice(groups, func(i, j int) bool {
		return strings.Join(groups[i].Terms, "\x00") < strings.Join(groups[j].Terms, "\x00")
	})
}

func loadSearchSynonymGroups(ctx context.Context, pool *pgxpool.Pool, schemaName, tableName string) (normalizedSearchSynonymGroups, error) {
	rows, err := pool.Query(ctx, `
		SELECT group_id::text, term
		FROM _ayb_search_synonyms
		WHERE schema_name = $1 AND table_name = $2
		ORDER BY group_id::text, term
	`, schemaName, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groupsByID := make(map[string][]string)
	for rows.Next() {
		var groupID string
		var term string
		if err := rows.Scan(&groupID, &term); err != nil {
			return nil, err
		}
		groupsByID[groupID] = append(groupsByID[groupID], term)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	groups := make(normalizedSearchSynonymGroups, 0, len(groupsByID))
	for _, terms := range groupsByID {
		sort.Strings(terms)
		groups = append(groups, searchSynonymGroup{Terms: terms})
	}
	sortSearchSynonymGroups(groups)
	return groups, nil
}

func replaceSearchSynonymGroups(ctx context.Context, pool *pgxpool.Pool, schemaName, tableName string, groups normalizedSearchSynonymGroups) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, `
		DELETE FROM _ayb_search_synonyms
		WHERE schema_name = $1 AND table_name = $2
	`, schemaName, tableName); err != nil {
		return err
	}
	if err := insertSearchSynonymGroups(ctx, tx, schemaName, tableName, groups); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func insertSearchSynonymGroups(ctx context.Context, tx pgx.Tx, schemaName, tableName string, groups normalizedSearchSynonymGroups) error {
	for _, group := range groups {
		groupID := uuid.NewString()
		for _, term := range group.Terms {
			if _, err := tx.Exec(ctx, `
				INSERT INTO _ayb_search_synonyms (schema_name, table_name, group_id, term)
				VALUES ($1, $2, $3, $4)
			`, schemaName, tableName, groupID, term); err != nil {
				return err
			}
		}
	}
	return nil
}
