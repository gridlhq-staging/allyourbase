// Package searchsynonyms Stub summary for /Users/stuart/parallel_development/allyourbase_dev/jun05_pm_2_algolia_importer_cli/allyourbase_dev/internal/searchsynonyms/store.go.
package searchsynonyms

import (
	"context"
	"database/sql"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	deleteGroupsSQL = `
		DELETE FROM _ayb_search_synonyms
		WHERE schema_name = $1 AND table_name = $2
	`
	insertGroupSQL = `
		INSERT INTO _ayb_search_synonyms (schema_name, table_name, group_id, term)
		VALUES ($1, $2, $3, $4)
	`
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) Store {
	return Store{pool: pool}
}

func (s Store) LoadGroups(ctx context.Context, schemaName, tableName string) (Groups, error) {
	rows, err := s.pool.Query(ctx, `
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

	groups := make(Groups, 0, len(groupsByID))
	for _, terms := range groupsByID {
		sort.Strings(terms)
		groups = append(groups, Group{Terms: terms})
	}
	sortGroups(groups)
	return groups, nil
}

func (s Store) ReplaceGroups(ctx context.Context, schemaName, tableName string, groups Groups) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, deleteGroupsSQL, schemaName, tableName); err != nil {
		return err
	}
	if err := insertGroups(ctx, tx, schemaName, tableName, groups); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ReplaceGroupsSQLTx replaces one collection's synonym groups inside caller's tx.
func ReplaceGroupsSQLTx(ctx context.Context, tx *sql.Tx, schemaName, tableName string, groups Groups) error {
	if _, err := tx.ExecContext(ctx, deleteGroupsSQL, schemaName, tableName); err != nil {
		return err
	}
	return insertGroupsSQLTx(ctx, tx, schemaName, tableName, groups)
}

func insertGroups(ctx context.Context, tx pgx.Tx, schemaName, tableName string, groups Groups) error {
	return forEachGroupTerm(groups, func(groupID, term string) error {
		_, err := tx.Exec(ctx, insertGroupSQL, schemaName, tableName, groupID, term)
		return err
	})
}

func insertGroupsSQLTx(ctx context.Context, tx *sql.Tx, schemaName, tableName string, groups Groups) error {
	return forEachGroupTerm(groups, func(groupID, term string) error {
		_, err := tx.ExecContext(ctx, insertGroupSQL, schemaName, tableName, groupID, term)
		return err
	})
}

func forEachGroupTerm(groups Groups, insert func(groupID, term string) error) error {
	for _, group := range groups {
		groupID := uuid.NewString()
		for _, term := range group.Terms {
			if err := insert(groupID, term); err != nil {
				return err
			}
		}
	}
	return nil
}
