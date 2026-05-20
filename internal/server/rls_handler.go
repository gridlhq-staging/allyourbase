// Package server Provides HTTP handlers for managing PostgreSQL row-level security policies and applying storage RLS templates to tables.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RlsPolicy represents a row-level security policy on a table.
type RlsPolicy struct {
	TableSchema   string   `json:"tableSchema"`
	TableName     string   `json:"tableName"`
	PolicyName    string   `json:"policyName"`
	Command       string   `json:"command"`
	Permissive    string   `json:"permissive"`
	Roles         []string `json:"roles"`
	UsingExpr     *string  `json:"usingExpr"`
	WithCheckExpr *string  `json:"withCheckExpr"`
}

// RlsTableStatus indicates whether RLS is enabled on a table.
type RlsTableStatus struct {
	RlsEnabled bool `json:"rlsEnabled"`
	ForceRls   bool `json:"forceRls"`
}

// rlsQuerier abstracts database access for testing.
type rlsQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgxRow
	Query(ctx context.Context, sql string, args ...any) (pgxRows, error)
	Exec(ctx context.Context, sql string, args ...any) error
}

// pgxRow matches pgx's Row interface.
type pgxRow interface {
	Scan(dest ...any) error
}

// pgxRows matches pgx's Rows interface.
type pgxRows interface {
	Next() bool
	Scan(dest ...any) error
	Close()
	Err() error
}

// poolAdapter wraps pgxpool.Pool to satisfy rlsQuerier.
type poolAdapter struct {
	pool *pgxpool.Pool
}

func (a *poolAdapter) QueryRow(ctx context.Context, sql string, args ...any) pgxRow {
	return a.pool.QueryRow(ctx, sql, args...)
}

func (a *poolAdapter) Query(ctx context.Context, sql string, args ...any) (pgxRows, error) {
	rows, err := a.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (a *poolAdapter) Exec(ctx context.Context, sql string, args ...any) error {
	_, err := a.pool.Exec(ctx, sql, args...)
	return err
}

// identifierRE validates SQL identifiers (table/policy names).
var identifierRE = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
var bucketNameRE = regexp.MustCompile(`^[a-z0-9_-]+$`)

func isValidIdentifier(s string) bool {
	return identifierRE.MatchString(s)
}

func isValidBucketName(s string) bool {
	return len(s) > 0 && len(s) <= 63 && bucketNameRE.MatchString(s)
}

// parseRlsTableIdentifier parses a table identifier string in the form 'schema.table' or 'table', validates both parts, and returns the schema and table names separately.
func parseRlsTableIdentifier(raw string) (schema string, table string, err error) {
	if raw == "" {
		return "", "", errors.New("table name is required")
	}

	schema, table, hasSchema := strings.Cut(raw, ".")
	if !hasSchema {
		if !isValidIdentifier(raw) {
			return "", "", errors.New("invalid table name")
		}
		return "", raw, nil
	}
	if !isValidIdentifier(schema) {
		return "", "", errors.New("invalid schema name")
	}
	if !isValidIdentifier(table) {
		return "", "", errors.New("invalid table name")
	}
	return schema, table, nil
}

func buildQualifiedTableSQL(schema string, table string) string {
	if schema == "" {
		return sqlutil.QuoteIdent(table)
	}
	return sqlutil.QuoteQualifiedName(schema, table)
}

// isSafePolicyExpression performs a minimal guard against stacked SQL statements.
// RLS expressions are SQL snippets, so we cannot heavily parse/transform them here,
// but we reject statement separators and comment tokens that can break out of the
// intended CREATE POLICY statement.
func isSafePolicyExpression(expr string) bool {
	return !strings.Contains(expr, ";") &&
		!strings.Contains(expr, "--") &&
		!strings.Contains(expr, "/*") &&
		!strings.Contains(expr, "*/") &&
		!strings.ContainsRune(expr, '\x00')
}

// handleListRlsPolicies returns all RLS policies, optionally filtered by table.
func handleListRlsPolicies(pool *pgxpool.Pool) http.HandlerFunc {
	q := &poolAdapter{pool: pool}
	return handleListRlsPoliciesWithQuerier(q)
}

// handleListRlsPoliciesWithQuerier returns an HTTP handler that lists all row-level security policies, optionally filtered by table name.
func handleListRlsPoliciesWithQuerier(q rlsQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawTable := chi.URLParam(r, "table")
		if rawTable == "" {
			policies, err := listPolicies(r.Context(), q, "", "")
			if err != nil {
				httputil.WriteError(w, http.StatusInternalServerError, "failed to list policies")
				return
			}
			httputil.WriteJSON(w, http.StatusOK, policies)
			return
		}
		schema, table, err := parseRlsTableIdentifier(rawTable)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		policies, err := listPolicies(r.Context(), q, schema, table)
		if err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to list policies")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, policies)
	}
}

// listPolicies queries the database for all RLS policies, optionally filtered by table name, returning a slice of RlsPolicy structs.
func listPolicies(ctx context.Context, q rlsQuerier, schema string, table string) ([]RlsPolicy, error) {
	query := `
		SELECT
			n.nspname AS table_schema,
			c.relname AS table_name,
			p.polname AS policy_name,
			CASE p.polcmd
				WHEN 'r' THEN 'SELECT'
				WHEN 'a' THEN 'INSERT'
				WHEN 'w' THEN 'UPDATE'
				WHEN 'd' THEN 'DELETE'
				WHEN '*' THEN 'ALL'
			END AS command,
			CASE WHEN p.polpermissive THEN 'PERMISSIVE' ELSE 'RESTRICTIVE' END AS permissive,
			COALESCE(ARRAY(
				SELECT rolname FROM pg_roles WHERE oid = ANY(p.polroles)
			), ARRAY[]::text[]) AS roles,
			pg_get_expr(p.polqual, p.polrelid) AS using_expr,
			pg_get_expr(p.polwithcheck, p.polrelid) AS with_check_expr
		FROM pg_policy p
		JOIN pg_class c ON c.oid = p.polrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
	`
	args := []any{}
	if table != "" && schema != "" {
		query += " AND n.nspname = $1 AND c.relname = $2"
		args = append(args, schema, table)
	} else if table != "" {
		query += " AND c.relname = $1"
		args = append(args, table)
	}
	query += " ORDER BY n.nspname, c.relname, p.polname"

	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query policies: %w", err)
	}
	defer rows.Close()

	var policies []RlsPolicy
	for rows.Next() {
		var pol RlsPolicy
		if err := rows.Scan(
			&pol.TableSchema, &pol.TableName, &pol.PolicyName,
			&pol.Command, &pol.Permissive, &pol.Roles,
			&pol.UsingExpr, &pol.WithCheckExpr,
		); err != nil {
			return nil, fmt.Errorf("scan policy: %w", err)
		}
		policies = append(policies, pol)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	if policies == nil {
		policies = []RlsPolicy{}
	}
	return policies, nil
}

// handleGetRlsStatus returns whether RLS is enabled on a table.
func handleGetRlsStatus(pool *pgxpool.Pool) http.HandlerFunc {
	q := &poolAdapter{pool: pool}
	return handleGetRlsStatusWithQuerier(q)
}

// handleGetRlsStatusWithQuerier returns an HTTP handler that retrieves whether RLS is enabled and force-enforced on a table.
func handleGetRlsStatusWithQuerier(q rlsQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawTable := chi.URLParam(r, "table")
		schema, table, err := parseRlsTableIdentifier(rawTable)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		var status RlsTableStatus
		query := `SELECT relrowsecurity, relforcerowsecurity
			 FROM pg_class c
			 JOIN pg_namespace n ON n.oid = c.relnamespace
			 WHERE c.relname = $1 AND n.nspname NOT IN ('pg_catalog', 'information_schema')`
		args := []any{table}
		if schema != "" {
			query += " AND n.nspname = $2"
			args = append(args, schema)
		}
		query += " LIMIT 1"
		err = q.QueryRow(r.Context(), query, args...).Scan(&status.RlsEnabled, &status.ForceRls)
		if err != nil {
			httputil.WriteError(w, http.StatusNotFound, "table not found")
			return
		}
		httputil.WriteJSON(w, http.StatusOK, status)
	}
}
