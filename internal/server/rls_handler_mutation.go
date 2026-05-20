// Package server provides RLS mutation and template admin handlers.
package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/sqlutil"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type createPolicyRequest struct {
	Table      string   `json:"table"`
	Schema     string   `json:"schema"`
	Name       string   `json:"name"`
	Command    string   `json:"command"`
	Permissive *bool    `json:"permissive"`
	Roles      []string `json:"roles"`
	Using      string   `json:"using"`
	WithCheck  string   `json:"withCheck"`
}

type storageTemplateRequest struct {
	Prefix string `json:"prefix"`
	Bucket string `json:"bucket"`
}

// handleCreateRlsPolicy creates a new RLS policy.
func handleCreateRlsPolicy(pool *pgxpool.Pool) http.HandlerFunc {
	q := &poolAdapter{pool: pool}
	return handleCreateRlsPolicyWithQuerier(q)
}

// handleCreateRlsPolicyWithQuerier returns an HTTP handler that creates a new RLS policy with the specified table, schema, name, command, permissive flag, roles, and optional USING and WITH CHECK expressions.
func handleCreateRlsPolicyWithQuerier(q rlsQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createPolicyRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}
		if req.Table == "" {
			httputil.WriteError(w, http.StatusBadRequest, "table is required")
			return
		}
		if req.Name == "" {
			httputil.WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if !isValidIdentifier(req.Table) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid table name")
			return
		}
		if !isValidIdentifier(req.Name) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid policy name")
			return
		}

		schema := req.Schema
		if schema == "" {
			schema = "public"
		}
		if !isValidIdentifier(schema) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid schema name")
			return
		}

		cmd := strings.ToUpper(req.Command)
		if cmd == "" {
			cmd = "ALL"
		}
		validCommands := map[string]bool{"ALL": true, "SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true}
		if !validCommands[cmd] {
			httputil.WriteErrorWithDocURL(w, http.StatusBadRequest, "command must be one of: ALL, SELECT, INSERT, UPDATE, DELETE",
				"https://allyourbase.io/guide/authentication#row-level-security-rls")
			return
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("CREATE POLICY %s ON %s", sqlutil.QuoteIdent(req.Name), buildQualifiedTableSQL(schema, req.Table)))
		if req.Permissive != nil && !*req.Permissive {
			sb.WriteString(" AS RESTRICTIVE")
		}
		sb.WriteString(fmt.Sprintf(" FOR %s", cmd))

		if len(req.Roles) > 0 {
			quoted := make([]string, len(req.Roles))
			for i, role := range req.Roles {
				if role == "PUBLIC" || role == "public" {
					quoted[i] = "PUBLIC"
				} else {
					if !isValidIdentifier(role) {
						httputil.WriteError(w, http.StatusBadRequest, "invalid role name: "+role)
						return
					}
					quoted[i] = sqlutil.QuoteIdent(role)
				}
			}
			sb.WriteString(fmt.Sprintf(" TO %s", strings.Join(quoted, ", ")))
		}

		if req.Using != "" {
			if !isSafePolicyExpression(req.Using) {
				httputil.WriteError(w, http.StatusBadRequest, "invalid USING expression")
				return
			}
			sb.WriteString(fmt.Sprintf(" USING (%s)", req.Using))
		}
		if req.WithCheck != "" {
			if !isSafePolicyExpression(req.WithCheck) {
				httputil.WriteError(w, http.StatusBadRequest, "invalid WITH CHECK expression")
				return
			}
			sb.WriteString(fmt.Sprintf(" WITH CHECK (%s)", req.WithCheck))
		}

		if err := q.Exec(r.Context(), sb.String()); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "failed to create policy: "+err.Error())
			return
		}
		httputil.WriteJSON(w, http.StatusCreated, map[string]string{"message": "policy created"})
	}
}

// handleDeleteRlsPolicy drops an RLS policy.
func handleDeleteRlsPolicy(pool *pgxpool.Pool) http.HandlerFunc {
	q := &poolAdapter{pool: pool}
	return handleDeleteRlsPolicyWithQuerier(q)
}

// handleDeleteRlsPolicyWithQuerier returns an HTTP handler that drops an RLS policy from a table, validating the table and policy names before execution.
func handleDeleteRlsPolicyWithQuerier(q rlsQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawTable := chi.URLParam(r, "table")
		policy := chi.URLParam(r, "policy")

		if rawTable == "" || policy == "" {
			httputil.WriteError(w, http.StatusBadRequest, "table and policy name are required")
			return
		}
		schema, table, err := parseRlsTableIdentifier(rawTable)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if !isValidIdentifier(policy) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid policy name")
			return
		}

		stmt := fmt.Sprintf("DROP POLICY %s ON %s", sqlutil.QuoteIdent(policy), buildQualifiedTableSQL(schema, table))
		if err := q.Exec(r.Context(), stmt); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "failed to drop policy: "+err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleApplyStorageObjectsTemplate(pool *pgxpool.Pool) http.HandlerFunc {
	q := &poolAdapter{pool: pool}
	return handleApplyStorageObjectsTemplateWithQuerier(q)
}

// handleApplyStorageObjectsTemplateWithQuerier returns an HTTP handler that applies a named storage RLS template to the storage objects table by generating and executing the required SQL statements.
func handleApplyStorageObjectsTemplateWithQuerier(q rlsQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		templateName := chi.URLParam(r, "template")
		if templateName == "" {
			httputil.WriteError(w, http.StatusBadRequest, "template is required")
			return
		}

		var req storageTemplateRequest
		if !httputil.DecodeJSON(w, r, &req) {
			return
		}

		if req.Prefix == "" {
			req.Prefix = "storage_policy"
		}
		if !isValidIdentifier(req.Prefix) {
			httputil.WriteError(w, http.StatusBadRequest, "invalid prefix")
			return
		}

		stmts, err := storageObjectsTemplateStatements(templateName, req)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		for _, stmt := range stmts {
			if err := q.Exec(r.Context(), stmt); err != nil {
				httputil.WriteError(w, http.StatusBadRequest, "failed to apply storage template: "+err.Error())
				return
			}
		}

		httputil.WriteJSON(w, http.StatusCreated, map[string]string{"message": "storage RLS template applied"})
	}
}

// storageObjectsTemplateStatements generates SQL statements to enable RLS and apply a named template (user-own-files, public-read-auth-write, or bucket-scoped) to the storage objects table.
func storageObjectsTemplateStatements(templateName string, req storageTemplateRequest) ([]string, error) {
	prefix := req.Prefix
	bucketLiteral := strings.ReplaceAll(req.Bucket, "'", "''")

	stmts := []string{`ALTER TABLE "_ayb_storage_objects" ENABLE ROW LEVEL SECURITY`}

	switch templateName {
	case "user-own-files":
		stmts = append(stmts,
			fmt.Sprintf(`CREATE POLICY %q ON "_ayb_storage_objects" AS PERMISSIVE FOR ALL TO "ayb_authenticated" USING (user_id = auth.uid()) WITH CHECK (user_id = auth.uid())`, prefix+"_all"),
		)
		return stmts, nil
	case "public-read-auth-write":
		stmts = append(stmts,
			fmt.Sprintf(`CREATE POLICY %q ON "_ayb_storage_objects" AS PERMISSIVE FOR SELECT TO "ayb_authenticated" USING (true)`, prefix+"_select"),
			fmt.Sprintf(`CREATE POLICY %q ON "_ayb_storage_objects" AS PERMISSIVE FOR INSERT TO "ayb_authenticated" WITH CHECK (auth.uid() IS NOT NULL)`, prefix+"_insert"),
			fmt.Sprintf(`CREATE POLICY %q ON "_ayb_storage_objects" AS PERMISSIVE FOR UPDATE TO "ayb_authenticated" USING (auth.uid() IS NOT NULL) WITH CHECK (auth.uid() IS NOT NULL)`, prefix+"_update"),
			fmt.Sprintf(`CREATE POLICY %q ON "_ayb_storage_objects" AS PERMISSIVE FOR DELETE TO "ayb_authenticated" USING (auth.uid() IS NOT NULL)`, prefix+"_delete"),
		)
		return stmts, nil
	case "bucket-scoped":
		if req.Bucket == "" {
			return nil, fmt.Errorf("bucket is required for bucket-scoped template")
		}
		if !isValidBucketName(req.Bucket) {
			return nil, fmt.Errorf("invalid bucket name")
		}
		stmts = append(stmts,
			fmt.Sprintf(`CREATE POLICY %q ON "_ayb_storage_objects" AS PERMISSIVE FOR ALL TO "ayb_authenticated" USING (bucket = '%s' AND user_id = auth.uid()) WITH CHECK (bucket = '%s' AND user_id = auth.uid())`, prefix+"_all", bucketLiteral, bucketLiteral),
		)
		return stmts, nil
	default:
		return nil, fmt.Errorf("unknown storage template: %s", templateName)
	}
}

// handleEnableRls enables RLS on a table.
func handleEnableRls(pool *pgxpool.Pool) http.HandlerFunc {
	q := &poolAdapter{pool: pool}
	return handleEnableRlsWithQuerier(q)
}

// handleEnableRlsWithQuerier returns an HTTP handler that enables row-level security on a table after validating the table name.
func handleEnableRlsWithQuerier(q rlsQuerier) http.HandlerFunc {
	return handleSetRlsStateWithQuerier(q, true)
}

// handleDisableRls disables RLS on a table.
func handleDisableRls(pool *pgxpool.Pool) http.HandlerFunc {
	q := &poolAdapter{pool: pool}
	return handleDisableRlsWithQuerier(q)
}

// handleDisableRlsWithQuerier returns an HTTP handler that disables row-level security on a table after validating the table name.
func handleDisableRlsWithQuerier(q rlsQuerier) http.HandlerFunc {
	return handleSetRlsStateWithQuerier(q, false)
}

// handleSetRlsStateWithQuerier returns an HTTP handler that enables or disables row-level security on a table based on the enable parameter, after validating the table name.
func handleSetRlsStateWithQuerier(q rlsQuerier, enable bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawTable := chi.URLParam(r, "table")
		schema, table, err := parseRlsTableIdentifier(rawTable)
		if err != nil {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}

		action := "disable"
		sqlAction := "DISABLE"
		if enable {
			action = "enable"
			sqlAction = "ENABLE"
		}

		stmt := fmt.Sprintf("ALTER TABLE %s %s ROW LEVEL SECURITY", buildQualifiedTableSQL(schema, table), sqlAction)
		if err := q.Exec(r.Context(), stmt); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, fmt.Sprintf("failed to %s RLS: %s", action, err.Error()))
			return
		}
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"message": fmt.Sprintf("RLS %sd on %s", action, table)})
	}
}
