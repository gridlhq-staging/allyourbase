//go:build integration

package sbmigrate

import (
	"context"
	"database/sql"
	"testing"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/testutil"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestE2E_FunctionMigration(t *testing.T) {
	sourceURL := createIsolatedDatabaseURL(t, sharedPG.ConnString, "sb_src_functions")
	targetURL := createIsolatedDatabaseURL(t, sharedPG.ConnString, "sb_tgt_functions")
	defer dropIsolatedDatabase(t, sharedPG.ConnString, sourceURL)
	defer dropIsolatedDatabase(t, sharedPG.ConnString, targetURL)

	sourceDB := openFunctionTestDatabase(t, sourceURL)
	defer sourceDB.Close()
	targetDB := openFunctionTestDatabase(t, targetURL)
	defer targetDB.Close()

	seedFunctionMigrationSource(t, sourceDB)
	setupTargetAYBSchemaForURL(t, targetURL)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:   sourceURL,
		TargetURL:   targetURL,
		Force:       true,
		SkipOAuth:   true,
		SkipRLS:     true,
		SkipStorage: true,
		Progress:    migrate.NopReporter{},
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	report, err := migrator.Analyze(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 5, report.Functions)

	stats, err := migrator.Migrate(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 1, stats.Tables)
	testutil.Equal(t, 0, stats.Views)
	testutil.Equal(t, 3, stats.Functions)
	testutil.Equal(t, 0, stats.Triggers)
	testutil.Equal(t, 2, stats.Skipped)
	testutil.Equal(t, 2, len(stats.SkippedFunctions))

	skippedIdentity := FunctionIdentity{SchemaName: "public", Name: "excluded_auth_reference"}
	testutil.Equal(
		t,
		"function definition references excluded schema auth",
		stats.SkippedFunctions[skippedIdentity.Key()],
	)
	delimiterCollisionIdentity := FunctionIdentity{
		SchemaName:        "public",
		Name:              "excluded_auth_reference_with_delimiter_collision",
		IdentityArguments: "label text",
	}
	testutil.Equal(
		t,
		"function definition references excluded schema auth",
		stats.SkippedFunctions[delimiterCollisionIdentity.Key()],
	)

	assertFunctionDefinitionParity(t, sourceDB, targetDB, "public.plpgsql_increment(integer)")
	assertFunctionDefinitionParity(t, sourceDB, targetDB, "public.sql_answer()")
	assertFunctionDefinitionParity(t, sourceDB, targetDB, "billing.sql_label()")
	assertFunctionResult(t, targetDB, `SELECT public.plpgsql_increment(4)`, 5)
	assertFunctionResult(t, targetDB, `SELECT public.sql_answer()`, 42)
	assertFunctionResult(t, targetDB, `SELECT billing.sql_label()`, "billing")
	assertFunctionResult(t, targetDB, `INSERT INTO function_default_specimen DEFAULT VALUES RETURNING value`, 5)
	assertFunctionAbsent(t, targetDB, "public.excluded_auth_reference()")
	assertFunctionAbsent(t, targetDB, "public.excluded_auth_reference_with_delimiter_collision(text)")
}

func TestE2E_TriggerMigration(t *testing.T) {
	sourceURL := createIsolatedDatabaseURL(t, sharedPG.ConnString, "sb_src_triggers")
	targetURL := createIsolatedDatabaseURL(t, sharedPG.ConnString, "sb_tgt_triggers")
	defer dropIsolatedDatabase(t, sharedPG.ConnString, sourceURL)
	defer dropIsolatedDatabase(t, sharedPG.ConnString, targetURL)

	sourceDB := openFunctionTestDatabase(t, sourceURL)
	defer sourceDB.Close()
	targetDB := openFunctionTestDatabase(t, targetURL)
	defer targetDB.Close()

	eventTriggerCreated := seedTriggerMigrationSource(t, sourceDB)
	setupTargetAYBSchemaForURL(t, targetURL)
	seedPreexistingPartitionCloneTarget(t, targetDB)

	progress := &phaseRecorder{}
	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:   sourceURL,
		TargetURL:   targetURL,
		Force:       true,
		SkipStorage: true,
		Progress:    progress,
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	report, err := migrator.Analyze(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 8, report.Tables)
	testutil.Equal(t, 1, report.AuthUsers)
	testutil.Equal(t, 1, report.OAuthLinks)
	testutil.Equal(t, 0, report.RLSPolicies)
	testutil.Equal(t, 4, report.Records)
	testutil.Equal(t, 5, report.Functions)
	testutil.Equal(t, expectedTriggerCatalogCount(eventTriggerCreated), report.Triggers)

	stats, err := migrator.Migrate(context.Background())
	testutil.NoError(t, err)
	progress.AssertAfter(t, "Triggers", "Data", "Auth users", "OAuth", "RLS policies")
	testutil.Equal(t, 8, stats.Tables)
	testutil.Equal(t, 4, stats.Records)
	testutil.Equal(t, 1, stats.Users)
	testutil.Equal(t, 1, stats.OAuthLinks)
	testutil.Equal(t, 0, stats.Policies)
	testutil.Equal(t, 3, stats.Functions)
	testutil.Equal(t, 5, stats.Triggers)
	testutil.Equal(t, expectedSkippedTriggerMigrationCount(eventTriggerCreated), stats.Skipped)
	testutil.Equal(t, 2, len(stats.SkippedFunctions))
	testutil.Equal(t, expectedSkippedTriggerCount(eventTriggerCreated), len(stats.SkippedTriggers))

	assertSkippedTrigger(t, stats, skippedHandlerTriggerIdentity(), "trigger handler public.skipped_trigger_handler() was not migrated")
	assertSkippedTrigger(t, stats, skippedPartitionedTriggerIdentity(), "trigger handler public.skipped_trigger_handler() was not migrated")
	assertSkippedTrigger(t, stats, constraintTriggerIdentity(), "constraint triggers are not supported")
	if eventTriggerCreated {
		assertSkippedTrigger(t, stats, eventTriggerIdentity(), "event triggers are not supported")
	}

	assertTriggerDefinitionParity(t, sourceDB, targetDB, "public", "trigger_specimens", "trigger_specimens_before_insert")
	assertTriggerDefinitionParity(t, sourceDB, targetDB, "public", "trigger_specimens", "trigger_specimens_before_update")
	assertTriggerDefinitionParity(t, sourceDB, targetDB, "public", "disabled_trigger_specimens", "disabled_before_insert")
	assertTriggerDefinitionParity(t, sourceDB, targetDB, "public", "partitioned_trigger_specimens", "partitioned_before_insert")
	assertTriggerDefinitionParity(t, sourceDB, targetDB, "public", "preexisting_partitioned_trigger_specimens", "preexisting_partitioned_before_insert")
	assertTriggerEnabledParity(t, sourceDB, targetDB, "public", "disabled_trigger_specimens", "disabled_before_insert")
	assertTriggerAbsent(t, targetDB, "public", "trigger_specimens", "trigger_specimens_constraint")
	assertTriggerAbsent(t, targetDB, "public", "skipped_handler_specimens", "skipped_handler_before_insert")
	assertSkippedPartitionTriggerAbsent(t, targetDB)

	assertTriggerOrderingGuard(t, targetDB)
	assertPartitionedRowCopiedOnce(t, targetDB)
	assertValidationSummaryHasNoRecordMismatch(t, report, stats)
	assertMigratedTriggerSideEffects(t, targetDB)
	assertDisabledTriggerRemainsInert(t, targetDB)
	assertPartitionTriggerClones(t, sourceDB, targetDB)
	assertExcludedPartitionRowNotCopied(t, targetDB)
	assertPreexistingPartitionTriggerCloneState(t, sourceDB, targetDB)
}

func TestE2E_OrdinaryInheritanceMigrationDoesNotEmitPartitionDDL(t *testing.T) {
	sourceURL := createIsolatedDatabaseURL(t, sharedPG.ConnString, "sb_src_inherits")
	targetURL := createIsolatedDatabaseURL(t, sharedPG.ConnString, "sb_tgt_inherits")
	defer dropIsolatedDatabase(t, sharedPG.ConnString, sourceURL)
	defer dropIsolatedDatabase(t, sharedPG.ConnString, targetURL)

	sourceDB := openFunctionTestDatabase(t, sourceURL)
	defer sourceDB.Close()
	targetDB := openFunctionTestDatabase(t, targetURL)
	defer targetDB.Close()

	seedOrdinaryInheritanceSource(t, sourceDB)
	setupTargetAYBSchemaForURL(t, targetURL)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:   sourceURL,
		TargetURL:   targetURL,
		Force:       true,
		SkipOAuth:   true,
		SkipRLS:     true,
		SkipStorage: true,
		Progress:    migrate.NopReporter{},
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 2, stats.Tables)
	assertTableExists(t, targetDB, "public", "ordinary_parent", true)
	assertTableExists(t, targetDB, "public", "ordinary_child", true)
}

func TestE2E_DataCopySkipDoesNotSuppressCreatedTableTrigger(t *testing.T) {
	sourceURL := createIsolatedDatabaseURL(t, sharedPG.ConnString, "sb_src_data_skip_trigger")
	targetURL := createIsolatedDatabaseURL(t, sharedPG.ConnString, "sb_tgt_data_skip_trigger")
	defer dropIsolatedDatabase(t, sharedPG.ConnString, sourceURL)
	defer dropIsolatedDatabase(t, sharedPG.ConnString, targetURL)

	sourceDB := openFunctionTestDatabase(t, sourceURL)
	defer sourceDB.Close()
	targetDB := openFunctionTestDatabase(t, targetURL)
	defer targetDB.Close()

	seedDataSkipTriggerSource(t, sourceDB)
	setupTargetAYBSchemaForURL(t, targetURL)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:   sourceURL,
		TargetURL:   targetURL,
		Force:       true,
		SkipOAuth:   true,
		SkipRLS:     true,
		SkipStorage: true,
		Progress:    migrate.NopReporter{},
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	stats, err := migrator.Migrate(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 2, stats.Tables)
	testutil.Equal(t, 1, stats.Functions)
	testutil.Equal(t, 1, stats.Triggers)
	testutil.Equal(t, 1, stats.Skipped)
	testutil.True(t, stats.SkippedTables[TableInfo{SchemaName: "public", Name: "data_skip_children"}.TableKey()] != "", "data-copy skip must remain reported")

	assertTriggerDefinitionParity(t, sourceDB, targetDB, "public", "data_skip_children", "data_skip_children_before_insert")
	assertDataSkippedTableTriggerFires(t, targetDB)
}

func TestE2E_FunctionMigrationSkipsPreSchemaCompositeSignatures(t *testing.T) {
	sourceURL := createIsolatedDatabaseURL(t, sharedPG.ConnString, "sb_src_function_composite")
	targetURL := createIsolatedDatabaseURL(t, sharedPG.ConnString, "sb_tgt_function_composite")
	defer dropIsolatedDatabase(t, sharedPG.ConnString, sourceURL)
	defer dropIsolatedDatabase(t, sharedPG.ConnString, targetURL)

	sourceDB := openFunctionTestDatabase(t, sourceURL)
	defer sourceDB.Close()
	targetDB := openFunctionTestDatabase(t, targetURL)
	defer targetDB.Close()

	seedCompositeSignatureFunctionSource(t, sourceDB)
	setupTargetAYBSchemaForURL(t, targetURL)

	migrator, err := NewMigrator(MigrationOptions{
		SourceURL:   sourceURL,
		TargetURL:   targetURL,
		Force:       true,
		SkipOAuth:   true,
		SkipRLS:     true,
		SkipStorage: true,
		Progress:    migrate.NopReporter{},
	})
	testutil.NoError(t, err)
	defer migrator.Close()

	report, err := migrator.Analyze(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 3, report.Functions)

	stats, err := migrator.Migrate(context.Background())
	testutil.NoError(t, err)
	testutil.Equal(t, 1, stats.Tables)
	testutil.Equal(t, 1, stats.Views)
	testutil.Equal(t, 0, stats.Functions)
	testutil.Equal(t, 0, stats.Triggers)
	testutil.Equal(t, 3, stats.Skipped)
	testutil.Equal(t, 3, len(stats.SkippedFunctions))

	skippedIdentity := FunctionIdentity{SchemaName: "billing", Name: "latest_invoice"}
	testutil.Equal(
		t,
		"function signature references table-defined composite type billing.invoice",
		stats.SkippedFunctions[skippedIdentity.Key()],
	)
	assertFunctionAbsent(t, targetDB, "billing.latest_invoice()")
	skippedViewIdentity := FunctionIdentity{
		SchemaName:        "billing",
		Name:              "invoice_view_total",
		IdentityArguments: "billing.invoice_view",
	}
	testutil.Equal(
		t,
		"function signature references view-defined composite type billing.invoice_view",
		stats.SkippedFunctions[skippedViewIdentity.Key()],
	)
	assertFunctionAbsent(t, targetDB, "billing.invoice_view_total(billing.invoice_view)")
	skippedMaterializedViewIdentity := FunctionIdentity{
		SchemaName:        "billing",
		Name:              "invoice_summary_total",
		IdentityArguments: "billing.invoice_summary",
	}
	testutil.Equal(
		t,
		"function signature references view-defined composite type billing.invoice_summary",
		stats.SkippedFunctions[skippedMaterializedViewIdentity.Key()],
	)
	assertFunctionAbsent(t, targetDB, "billing.invoice_summary_total(billing.invoice_summary)")
	assertTableExists(t, targetDB, "billing", "invoice", true)
}

func openFunctionTestDatabase(t *testing.T, databaseURL string) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", databaseURL)
	testutil.NoError(t, err)
	return db
}

func seedFunctionMigrationSource(t *testing.T, sourceDB *sql.DB) {
	t.Helper()
	_, err := sourceDB.Exec(`
		CREATE SCHEMA auth;
		CREATE SCHEMA billing;
		CREATE TABLE auth.users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email TEXT,
			encrypted_password TEXT,
			email_confirmed_at TIMESTAMPTZ,
			raw_user_meta_data JSONB NOT NULL DEFAULT '{}'::jsonb,
			raw_app_meta_data JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ,
			is_anonymous BOOLEAN DEFAULT false
		);

		CREATE FUNCTION public.plpgsql_increment(integer)
		RETURNS integer
		LANGUAGE plpgsql
		IMMUTABLE
		AS $$ BEGIN RETURN $1 + 1; END; $$;

		CREATE FUNCTION public.sql_answer()
		RETURNS integer
		LANGUAGE sql
		IMMUTABLE
		AS $$ SELECT 42; $$;

		CREATE FUNCTION billing.sql_label()
		RETURNS text
		LANGUAGE sql
		IMMUTABLE
		AS $$ SELECT 'billing'::text; $$;

		CREATE FUNCTION public.excluded_auth_reference()
		RETURNS uuid
		LANGUAGE plpgsql
		AS $$ BEGIN RETURN auth.uid(); END; $$;

		CREATE FUNCTION public.excluded_auth_reference_with_delimiter_collision(
			label text DEFAULT 'AS $$'::text
		)
		RETURNS uuid
		LANGUAGE plpgsql
		AS $function$ BEGIN RETURN auth.uid(); END; $function$;

		CREATE TABLE public.function_default_specimen (
			value integer NOT NULL DEFAULT public.plpgsql_increment(4)
		);
	`)
	testutil.NoError(t, err)
}

func seedCompositeSignatureFunctionSource(t *testing.T, sourceDB *sql.DB) {
	t.Helper()
	_, err := sourceDB.Exec(`
		CREATE SCHEMA auth;
		CREATE SCHEMA billing;
		CREATE TABLE auth.users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email TEXT,
			encrypted_password TEXT,
			email_confirmed_at TIMESTAMPTZ,
			raw_user_meta_data JSONB NOT NULL DEFAULT '{}'::jsonb,
			raw_app_meta_data JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ,
			is_anonymous BOOLEAN DEFAULT false
		);
		CREATE TABLE billing.invoice (
			id bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
			amount integer NOT NULL
		);
		CREATE FUNCTION billing.latest_invoice()
		RETURNS billing.invoice
		LANGUAGE sql
		STABLE
		AS $$ SELECT * FROM billing.invoice ORDER BY id DESC LIMIT 1; $$;
		CREATE VIEW billing.invoice_view AS
			SELECT id, amount FROM billing.invoice;
		CREATE FUNCTION billing.invoice_view_total(billing.invoice_view)
		RETURNS integer
		LANGUAGE sql
		IMMUTABLE
		AS $$ SELECT $1.amount; $$;
		CREATE MATERIALIZED VIEW billing.invoice_summary AS
			SELECT sum(amount)::integer AS total FROM billing.invoice;
		CREATE FUNCTION billing.invoice_summary_total(billing.invoice_summary)
		RETURNS integer
		LANGUAGE sql
		IMMUTABLE
		AS $$ SELECT $1.total; $$;
	`)
	testutil.NoError(t, err)
}

func assertFunctionDefinitionParity(t *testing.T, sourceDB, targetDB *sql.DB, identity string) {
	t.Helper()
	sourceDefinition := readFunctionDefinition(t, sourceDB, identity)
	targetDefinition := readFunctionDefinition(t, targetDB, identity)
	testutil.Equal(t, sourceDefinition, targetDefinition)
}

func readFunctionDefinition(t *testing.T, db *sql.DB, identity string) string {
	t.Helper()
	var definition string
	err := db.QueryRow(`SELECT pg_get_functiondef($1::regprocedure)`, identity).Scan(&definition)
	testutil.NoError(t, err)
	return definition
}

func assertFunctionResult[T comparable](t *testing.T, db *sql.DB, query string, expected T) {
	t.Helper()
	var actual T
	err := db.QueryRow(query).Scan(&actual)
	testutil.NoError(t, err)
	testutil.Equal(t, expected, actual)
}

func assertFunctionAbsent(t *testing.T, db *sql.DB, identity string) {
	t.Helper()
	var absent bool
	err := db.QueryRow(`SELECT to_regprocedure($1) IS NULL`, identity).Scan(&absent)
	testutil.NoError(t, err)
	testutil.True(t, absent, "skipped function must not exist in target")
}
