package sbmigrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
)

type functionCatalogAction uint8

const (
	functionCatalogIgnore functionCatalogAction = iota
	functionCatalogMigrate
	functionCatalogSkip
)

type functionCatalogEntry struct {
	Identity                  FunctionIdentity
	Definition                string
	Kind                      string
	Language                  string
	ExtensionOwned            bool
	OperatorImplementation    bool
	CompositeTypeDependency   string
	CompositeTypeRelationKind string
}

type functionCatalogClassification struct {
	Action functionCatalogAction
	Reason string
}

func loadFunctionCatalog(ctx context.Context, db *sql.DB) ([]functionCatalogEntry, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			n.nspname,
			p.proname,
			pg_get_function_identity_arguments(p.oid),
			COALESCE(
				CASE WHEN p.prokind IN ('f', 'p') THEN pg_get_functiondef(p.oid) END,
				''
			),
			p.prokind::text,
			l.lanname,
			EXISTS (
				SELECT 1
				FROM pg_depend d
				JOIN pg_extension e ON e.oid = d.refobjid
				WHERE d.classid = 'pg_proc'::regclass
				  AND d.objid = p.oid
				  AND d.refclassid = 'pg_extension'::regclass
				  AND d.deptype = 'e'
			),
			EXISTS (
				SELECT 1
				FROM pg_operator o
				WHERE o.oprcode = p.oid
			),
			COALESCE(composite_type_dependency.qualified_name, ''),
			COALESCE(composite_type_dependency.relation_kind, '')
		FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		JOIN pg_language l ON l.oid = p.prolang
		LEFT JOIN LATERAL (
			SELECT
				tn.nspname || '.' || c.relname AS qualified_name,
				c.relkind::text AS relation_kind
			FROM (
				SELECT p.prorettype AS type_oid
				UNION
				SELECT unnest(p.proargtypes::oid[]) AS type_oid
				UNION
				SELECT unnest(COALESCE(p.proallargtypes, ARRAY[]::oid[])) AS type_oid
			) signature_type
			JOIN pg_type declared_type ON declared_type.oid = signature_type.type_oid
			JOIN pg_type t
			  ON t.oid = CASE
				WHEN declared_type.typelem <> 0 THEN declared_type.typelem
				ELSE declared_type.oid
			  END
			JOIN pg_class c ON c.oid = t.typrelid
			JOIN pg_namespace tn ON tn.oid = t.typnamespace
			WHERE t.typtype = 'c'
			  AND tn.nspname NOT IN ('pg_catalog', 'information_schema')
			ORDER BY tn.nspname, c.relname
			LIMIT 1
		) composite_type_dependency ON true
		ORDER BY n.nspname, p.proname, pg_get_function_identity_arguments(p.oid)
	`)
	if err != nil {
		return nil, fmt.Errorf("querying function catalog: %w", err)
	}
	defer rows.Close()

	var entries []functionCatalogEntry
	for rows.Next() {
		var entry functionCatalogEntry
		if err := rows.Scan(
			&entry.Identity.SchemaName,
			&entry.Identity.Name,
			&entry.Identity.IdentityArguments,
			&entry.Definition,
			&entry.Kind,
			&entry.Language,
			&entry.ExtensionOwned,
			&entry.OperatorImplementation,
			&entry.CompositeTypeDependency,
			&entry.CompositeTypeRelationKind,
		); err != nil {
			return nil, fmt.Errorf("scanning function catalog: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading function catalog: %w", err)
	}
	return entries, nil
}

func classifyFunctionCatalogEntry(entry functionCatalogEntry) functionCatalogClassification {
	if functionCatalogEntryMalformed(entry) {
		return skippedFunctionClassification("function catalog row is malformed")
	}
	if isPostgresCatalogSchema(entry.Identity.SchemaName) {
		return functionCatalogClassification{Action: functionCatalogIgnore}
	}
	if !isAdmittedUserSchema(entry.Identity.SchemaName) {
		return skippedFunctionClassification(
			"function belongs to excluded schema " + entry.Identity.SchemaName,
		)
	}
	if entry.ExtensionOwned {
		return skippedFunctionClassification("function is owned by an extension")
	}

	switch entry.Kind {
	case "a":
		return skippedFunctionClassification("aggregate functions are not supported")
	case "w":
		return skippedFunctionClassification("window functions are not supported")
	case "p":
		return skippedFunctionClassification("procedures are not supported")
	case "f":
	default:
		return skippedFunctionClassification("function kind " + entry.Kind + " is not supported")
	}

	if entry.OperatorImplementation {
		return skippedFunctionClassification("operator implementation functions are not supported")
	}
	if entry.Language != "plpgsql" && entry.Language != "sql" {
		return skippedFunctionClassification("function language " + entry.Language + " is not supported")
	}
	if entry.Definition == "" {
		return skippedFunctionClassification("function catalog row is malformed")
	}
	if entry.CompositeTypeDependency != "" {
		return classifyCompositeTypeDependency(entry)
	}
	// Body references are deliberately over-skipped: rewriting executable SQL
	// would silently change behavior and is less safe than a reason-bearing skip.
	if schema, found := excludedSchemaReference(entry.Definition); found {
		return skippedFunctionClassification(
			"function definition references excluded schema " + schema,
		)
	}
	return functionCatalogClassification{Action: functionCatalogMigrate}
}

func classifyCompositeTypeDependency(entry functionCatalogEntry) functionCatalogClassification {
	switch entry.CompositeTypeRelationKind {
	case "r", "p":
		return skippedFunctionClassification(
			"function signature references table-defined composite type " + entry.CompositeTypeDependency,
		)
	case "v", "m":
		return skippedFunctionClassification(
			"function signature references view-defined composite type " + entry.CompositeTypeDependency,
		)
	default:
		return skippedFunctionClassification(
			"function signature references pre-schema composite type " + entry.CompositeTypeDependency,
		)
	}
}

func functionCatalogEntryMalformed(entry functionCatalogEntry) bool {
	return entry.Identity.SchemaName == "" ||
		entry.Identity.Name == "" ||
		entry.Kind == "" ||
		entry.Language == ""
}

func isPostgresCatalogSchema(schema string) bool {
	return schema == "information_schema" || strings.HasPrefix(schema, "pg_")
}

func skippedFunctionClassification(reason string) functionCatalogClassification {
	return functionCatalogClassification{Action: functionCatalogSkip, Reason: reason}
}

func functionCatalogDenominator(entries []functionCatalogEntry) int {
	total := 0
	for _, entry := range entries {
		if classifyFunctionCatalogEntry(entry).Action != functionCatalogIgnore {
			total++
		}
	}
	return total
}

func functionSchemasForMigration(entries []functionCatalogEntry) []string {
	var schemas []string
	for _, entry := range entries {
		if classifyFunctionCatalogEntry(entry).Action == functionCatalogMigrate {
			schemas = append(schemas, entry.Identity.SchemaName)
		}
	}
	return schemas
}

func (m *Migrator) migrateFunctions(
	ctx context.Context,
	tx *sql.Tx,
	entries []functionCatalogEntry,
	phase migrate.Phase,
) (migratedFunctionSet, error) {
	totalItems := functionCatalogDenominator(entries)
	m.progress.StartPhase(phase, totalItems)
	fmt.Fprintln(m.output, "Migrating functions...")
	start := time.Now()

	// SQL functions can reference tables that are intentionally created after
	// this phase, so validation must be deferred until the complete schema exists.
	if _, err := tx.ExecContext(ctx, `SET LOCAL check_function_bodies = off`); err != nil {
		return nil, fmt.Errorf("disabling function body checks: %w", err)
	}

	migratedFunctions := make(migratedFunctionSet)
	processed := 0
	migrated := 0
	skipped := 0
	for _, entry := range entries {
		classification := classifyFunctionCatalogEntry(entry)
		switch classification.Action {
		case functionCatalogIgnore:
			continue
		case functionCatalogSkip:
			m.skipFunction(phase, entry.Identity, classification.Reason)
			skipped++
		case functionCatalogMigrate:
			if _, err := tx.ExecContext(ctx, entry.Definition); err != nil {
				return nil, fmt.Errorf("creating function %s: %w", entry.Identity.QualifiedName(), err)
			}
			migratedFunctions[entry.Identity.Key()] = struct{}{}
			m.stats.Functions++
			migrated++
			if m.verbose {
				fmt.Fprintf(m.output, "  CREATE FUNCTION %s\n", entry.Identity.QualifiedName())
			}
		}
		processed++
		m.progress.Progress(phase, processed, totalItems)
	}

	m.progress.CompletePhase(phase, processed, time.Since(start))
	fmt.Fprintf(m.output, "  ✓ %d functions migrated, %d skipped\n", migrated, skipped)
	return migratedFunctions, nil
}

func (m *Migrator) skipFunction(phase migrate.Phase, identity FunctionIdentity, reason string) {
	m.markSkippedFunction(identity, errors.New(reason))
	m.stats.Skipped++
	m.progress.Warn(fmt.Sprintf("skipping function %s: %s", identity.QualifiedName(), reason))
	if m.verbose {
		fmt.Fprintf(m.output, "  SKIP FUNCTION %s (%s)\n", identity.QualifiedName(), reason)
	}
}
