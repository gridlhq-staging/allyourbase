// Package realtime visibility.go owns per-record visibility proof: the
// SELECT-applicable RLS policy predicates, the PK-bound visibility queries, and
// the RLS-scoped execution seam they run under. CanSeeRecord in handler.go
// orchestrates these helpers; the rules themselves live here, next to their
// visibility_test.go owner.
package realtime

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/schema"
	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

// canSeeDeletedRecord applies the delete-visibility truth table. Delete
// filtering needs OldRecord because transports serialize Event.Record to clients
// while keeping OldRecord internal; without OldRecord, the handler cannot prove
// the deleted row was visible and fails closed. Missing OldRecord PK values and
// tables without RLS SELECT/ALL policies retain the established fail-open
// behavior. Otherwise it evaluates OldRecord against the table's
// SELECT-applicable UsingExpr policies under the request user's RLS context and
// delivers only when the deleted row would have been visible.
func canSeeDeletedRecord(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, tbl *schema.Table, oldRecord map[string]any, claims *auth.Claims) bool {
	if oldRecord == nil {
		return false
	}
	if !schema.RecordHasPrimaryKeyValues(tbl, oldRecord) {
		return true
	}

	predicate, enforce := deleteVisibilityPredicate(tbl)
	if !enforce {
		return true
	}

	query, args := buildDeletedVisibilityCheck(tbl, predicate, oldRecord)
	return runVisibilityCheck(ctx, pool, logger, claims, query, args)
}

// runVisibilityCheck executes a visibility query under the caller's RLS context and fails closed on any error.
func runVisibilityCheck(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger, claims *auth.Claims, query string, args []any) bool {
	tx, err := pool.Begin(ctx)
	if err != nil {
		logger.Error("rls filter: begin tx", "error", err)
		return false // fail closed
	}
	defer tx.Rollback(ctx)

	if err := auth.SetRLSContext(ctx, tx, claims); err != nil {
		logger.Error("rls filter: set rls context", "error", err)
		return false
	}

	var one int
	err = tx.QueryRow(ctx, query, args...).Scan(&one)
	return err == nil
}

// canProveRecordVisibility reports whether the table has the metadata needed to
// build a per-record visibility query that RLS can narrow safely for
// cross-tenant candidates: a primary key plus at least one SELECT- or
// ALL-applicable policy on an RLS-enabled table.
func canProveRecordVisibility(tbl *schema.Table) bool {
	return tbl != nil && len(tbl.PrimaryKey) > 0 && hasSelectApplicablePolicy(tbl)
}

func hasSelectApplicablePolicy(tbl *schema.Table) bool {
	if tbl == nil || !tbl.RLSEnabled {
		return false
	}
	for _, policy := range tbl.RLSPolicies {
		if isSelectApplicablePolicy(policy) {
			return true
		}
	}
	return false
}

func isSelectApplicablePolicy(policy *schema.RLSPolicy) bool {
	if policy == nil {
		return false
	}
	command := strings.ToUpper(strings.TrimSpace(policy.Command))
	return command == "ALL" || command == "SELECT"
}

// deleteVisibilityPredicate combines SELECT-applicable policies for evaluation against a deleted row.
func deleteVisibilityPredicate(tbl *schema.Table) (string, bool) {
	if !tbl.RLSEnabled {
		return "", false
	}
	var permissive []string
	var restrictive []string
	for _, policy := range tbl.RLSPolicies {
		if !isSelectApplicablePolicy(policy) {
			continue
		}
		expr := strings.TrimSpace(policy.UsingExpr)
		if expr == "" {
			expr = "TRUE"
		}
		if policy.Permissive {
			permissive = append(permissive, expr)
		} else {
			restrictive = append(restrictive, expr)
		}
	}
	if len(permissive) == 0 && len(restrictive) == 0 {
		return "", false
	}
	if len(permissive) == 0 {
		return "FALSE", true
	}
	clauses := append([]string{joinPolicyPredicates(permissive, " OR ")}, restrictive...)
	return joinPolicyPredicates(clauses, " AND "), true
}

func joinPolicyPredicates(predicates []string, sep string) string {
	wrapped := make([]string, len(predicates))
	for i, predicate := range predicates {
		wrapped[i] = "(" + predicate + ")"
	}
	return strings.Join(wrapped, sep)
}

// buildDeletedVisibilityCheck evaluates a deleted record as a typed VALUES row against an RLS predicate.
func buildDeletedVisibilityCheck(tbl *schema.Table, predicate string, record map[string]any) (string, []any) {
	columns := make([]string, 0, len(record))
	for column := range record {
		columns = append(columns, column)
	}
	sort.Strings(columns)

	args := make([]any, 0, len(columns))
	placeholders := make([]string, len(columns))
	quotedColumns := make([]string, len(columns))
	for i, column := range columns {
		placeholders[i] = deletedRecordPlaceholder(tbl, column, i+1)
		quotedColumns[i] = sqlutil.QuoteIdent(column)
		args = append(args, record[column])
	}
	query := fmt.Sprintf("SELECT 1 FROM (VALUES (%s)) AS %s (%s) WHERE %s",
		strings.Join(placeholders, ", "),
		sqlutil.QuoteIdent(tbl.Name),
		strings.Join(quotedColumns, ", "),
		predicate)
	return query, args
}

// buildVisibilityCheck builds a SELECT 1 query scoped to a row's PK.
// Returns ("", nil) if the record is missing any PK column value.
func buildVisibilityCheck(tbl *schema.Table, record map[string]any) (string, []any) {
	args := make([]any, 0, len(tbl.PrimaryKey))
	var sb strings.Builder
	sb.WriteString("SELECT 1 FROM ")
	sb.WriteString(sqlutil.QuoteQualifiedName(tbl.Schema, tbl.Name))
	sb.WriteString(" WHERE ")

	for i, pk := range tbl.PrimaryKey {
		v, ok := record[pk]
		if !ok {
			return "", nil
		}
		if i > 0 {
			sb.WriteString(" AND ")
		}
		sb.WriteString(sqlutil.QuoteIdent(pk))
		sb.WriteString(" = $")
		sb.WriteString(strconv.Itoa(i + 1))
		args = append(args, v)
	}
	return sb.String(), args
}
