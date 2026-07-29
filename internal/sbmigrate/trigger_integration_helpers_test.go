//go:build integration

package sbmigrate

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/testutil"
)

func seedTriggerMigrationSource(t *testing.T, sourceDB *sql.DB) bool {
	t.Helper()
	seedTriggerMigrationAuthAndTables(t, sourceDB)
	seedTriggerMigrationFunctionsAndTriggers(t, sourceDB)
	seedPreexistingPartitionCloneSource(t, sourceDB)
	return seedTriggerMigrationEventTrigger(t, sourceDB)
}

func seedTriggerMigrationAuthAndTables(t *testing.T, sourceDB *sql.DB) {
	t.Helper()
	_, err := sourceDB.Exec(`
		CREATE SCHEMA auth;
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
		CREATE TABLE auth.identities (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES auth.users(id),
			provider TEXT NOT NULL,
			identity_data JSONB NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);
		INSERT INTO auth.users (id, email, encrypted_password, email_confirmed_at)
		VALUES ('aaaaaaaa-0000-0000-0000-000000000101', 'trigger@example.com', '$2a$10$hash', NOW());
		INSERT INTO auth.identities (user_id, provider, identity_data)
		VALUES ('aaaaaaaa-0000-0000-0000-000000000101', 'google', '{"sub":"trigger-user","email":"trigger@example.com"}'::jsonb);

		CREATE FUNCTION auth.uid()
		RETURNS uuid
		LANGUAGE sql
		STABLE
		AS $$ SELECT 'aaaaaaaa-0000-0000-0000-000000000101'::uuid; $$;

		CREATE TABLE public.trigger_specimens (
			id integer PRIMARY KEY,
			value text NOT NULL,
			audit_count integer NOT NULL DEFAULT 0,
			updated_marker text NOT NULL DEFAULT ''
		);
		INSERT INTO public.trigger_specimens (id, value, audit_count, updated_marker)
		VALUES (1, 'source-preserved', 77, 'seeded');

		CREATE TABLE public.disabled_trigger_specimens (
			id integer PRIMARY KEY,
			value text NOT NULL
		);
		INSERT INTO public.disabled_trigger_specimens (id, value)
		VALUES (1, 'disabled-source');

		CREATE TABLE public.skipped_handler_specimens (
			id integer PRIMARY KEY,
			value text NOT NULL
		);
		INSERT INTO public.skipped_handler_specimens (id, value)
		VALUES (1, 'skipped-source');

		CREATE TABLE public.partitioned_trigger_specimens (
			id integer NOT NULL,
			value text NOT NULL
		) PARTITION BY RANGE (id);
		CREATE TABLE public.partitioned_trigger_specimens_low
		PARTITION OF public.partitioned_trigger_specimens
		FOR VALUES FROM (0) TO (100);
		CREATE TABLE auth.partitioned_trigger_specimens_high
		PARTITION OF public.partitioned_trigger_specimens
		FOR VALUES FROM (100) TO (200);
		INSERT INTO public.partitioned_trigger_specimens (id, value)
		VALUES
			(1, 'partition-source'),
			(101, 'excluded-partition-source');

		CREATE TABLE public.skipped_partitioned_trigger_specimens (
			id integer NOT NULL
		) PARTITION BY RANGE (id);
		CREATE TABLE public.skipped_partitioned_trigger_specimens_low
		PARTITION OF public.skipped_partitioned_trigger_specimens
		FOR VALUES FROM (0) TO (100);
	`)
	testutil.NoError(t, err)
}

func seedTriggerMigrationFunctionsAndTriggers(t *testing.T, sourceDB *sql.DB) {
	t.Helper()
	_, err := sourceDB.Exec(`
		CREATE FUNCTION public.apply_trigger_side_effect()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			IF TG_OP = 'INSERT' THEN
				NEW.value := NEW.value || ':inserted';
				NEW.audit_count := NEW.audit_count + 10;
			ELSIF TG_OP = 'UPDATE' THEN
				NEW.audit_count := OLD.audit_count + 1;
				NEW.updated_marker := 'updated';
			END IF;
			RETURN NEW;
		END;
		$$;
		CREATE FUNCTION public.disabled_trigger_handler()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			NEW.value := 'disabled-fired';
			RETURN NEW;
		END;
		$$;
		CREATE FUNCTION public.skipped_trigger_handler()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			PERFORM auth.uid();
			RETURN NEW;
		END;
		$$;
		CREATE FUNCTION public.ddl_event_handler()
		RETURNS event_trigger
		LANGUAGE plpgsql
		AS $$ BEGIN END; $$;

		CREATE TRIGGER trigger_specimens_before_insert
		BEFORE INSERT ON public.trigger_specimens
		FOR EACH ROW EXECUTE FUNCTION public.apply_trigger_side_effect();
		CREATE TRIGGER trigger_specimens_before_update
		BEFORE UPDATE ON public.trigger_specimens
		FOR EACH ROW EXECUTE FUNCTION public.apply_trigger_side_effect();
		CREATE TRIGGER disabled_before_insert
		BEFORE INSERT ON public.disabled_trigger_specimens
		FOR EACH ROW EXECUTE FUNCTION public.disabled_trigger_handler();
		ALTER TABLE public.disabled_trigger_specimens DISABLE TRIGGER disabled_before_insert;
		CREATE TRIGGER skipped_handler_before_insert
		BEFORE INSERT ON public.skipped_handler_specimens
		FOR EACH ROW EXECUTE FUNCTION public.skipped_trigger_handler();
		CREATE TRIGGER partitioned_before_insert
		BEFORE INSERT ON public.partitioned_trigger_specimens
		FOR EACH ROW EXECUTE FUNCTION public.apply_trigger_side_effect();
		ALTER TABLE public.partitioned_trigger_specimens_low DISABLE TRIGGER partitioned_before_insert;
		ALTER TABLE auth.partitioned_trigger_specimens_high DISABLE TRIGGER partitioned_before_insert;
		CREATE TRIGGER skipped_partitioned_before_insert
		BEFORE INSERT ON public.skipped_partitioned_trigger_specimens
		FOR EACH ROW EXECUTE FUNCTION public.skipped_trigger_handler();
		ALTER TABLE public.skipped_partitioned_trigger_specimens_low
		DISABLE TRIGGER skipped_partitioned_before_insert;
		CREATE CONSTRAINT TRIGGER trigger_specimens_constraint
		AFTER INSERT ON public.trigger_specimens
		DEFERRABLE INITIALLY DEFERRED
		FOR EACH ROW EXECUTE FUNCTION public.apply_trigger_side_effect();
	`)
	testutil.NoError(t, err)
}

func seedTriggerMigrationEventTrigger(t *testing.T, sourceDB *sql.DB) bool {
	t.Helper()
	_, err := sourceDB.Exec(`
		CREATE EVENT TRIGGER ddl_audit
		ON ddl_command_end
		EXECUTE FUNCTION public.ddl_event_handler();
	`)
	if err != nil {
		t.Logf("event trigger specimen not created: %v", err)
		return false
	}
	return true
}

func expectedTriggerCatalogCount(eventTriggerCreated bool) int {
	if eventTriggerCreated {
		return 9
	}
	return 8
}

func expectedSkippedTriggerCount(eventTriggerCreated bool) int {
	if eventTriggerCreated {
		return 4
	}
	return 3
}

func expectedSkippedTriggerMigrationCount(eventTriggerCreated bool) int {
	return 2 + expectedSkippedTriggerCount(eventTriggerCreated)
}

func skippedHandlerTriggerIdentity() TriggerIdentity {
	return TriggerIdentity{
		TableSchemaName:   "public",
		TableName:         "skipped_handler_specimens",
		Name:              "skipped_handler_before_insert",
		HandlerSchemaName: "public",
		HandlerName:       "skipped_trigger_handler",
	}
}

func skippedPartitionedTriggerIdentity() TriggerIdentity {
	return TriggerIdentity{
		TableSchemaName:   "public",
		TableName:         "skipped_partitioned_trigger_specimens",
		Name:              "skipped_partitioned_before_insert",
		HandlerSchemaName: "public",
		HandlerName:       "skipped_trigger_handler",
	}
}

func constraintTriggerIdentity() TriggerIdentity {
	return TriggerIdentity{
		TableSchemaName:   "public",
		TableName:         "trigger_specimens",
		Name:              "trigger_specimens_constraint",
		HandlerSchemaName: "public",
		HandlerName:       "apply_trigger_side_effect",
	}
}

func eventTriggerIdentity() TriggerIdentity {
	return TriggerIdentity{
		EventTrigger:      true,
		Name:              "ddl_audit",
		HandlerSchemaName: "public",
		HandlerName:       "ddl_event_handler",
	}
}

func assertSkippedTrigger(t *testing.T, stats *MigrationStats, identity TriggerIdentity, reason string) {
	t.Helper()
	testutil.Equal(t, reason, stats.SkippedTriggers[identity.Key()])
}

func assertTriggerDefinitionParity(t *testing.T, sourceDB, targetDB *sql.DB, schema, table, trigger string) {
	t.Helper()
	sourceDefinition := readTriggerDefinition(t, sourceDB, schema, table, trigger)
	targetDefinition := readTriggerDefinition(t, targetDB, schema, table, trigger)
	testutil.Equal(t, sourceDefinition, targetDefinition)
}

func readTriggerDefinition(t *testing.T, db *sql.DB, schema, table, trigger string) string {
	t.Helper()
	var definition string
	err := db.QueryRow(`
		SELECT pg_get_triggerdef(t.oid)
		FROM pg_trigger t
		JOIN pg_class c ON c.oid = t.tgrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relname = $2 AND t.tgname = $3
	`, schema, table, trigger).Scan(&definition)
	testutil.NoError(t, err)
	return definition
}

func assertTriggerEnabledParity(t *testing.T, sourceDB, targetDB *sql.DB, schema, table, trigger string) {
	t.Helper()
	sourceEnabled := readTriggerEnabled(t, sourceDB, schema, table, trigger)
	targetEnabled := readTriggerEnabled(t, targetDB, schema, table, trigger)
	testutil.Equal(t, sourceEnabled, targetEnabled)
}

func readTriggerEnabled(t *testing.T, db *sql.DB, schema, table, trigger string) string {
	t.Helper()
	var enabled string
	err := db.QueryRow(`
		SELECT t.tgenabled::text
		FROM pg_trigger t
		JOIN pg_class c ON c.oid = t.tgrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relname = $2 AND t.tgname = $3
	`, schema, table, trigger).Scan(&enabled)
	testutil.NoError(t, err)
	return enabled
}

func assertTriggerAbsent(t *testing.T, db *sql.DB, schema, table, trigger string) {
	t.Helper()
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM pg_trigger t
			JOIN pg_class c ON c.oid = t.tgrelid
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = $1 AND c.relname = $2 AND t.tgname = $3
		)
	`, schema, table, trigger).Scan(&exists)
	testutil.NoError(t, err)
	testutil.False(t, exists, "skipped trigger must not exist in target")
}

func assertTriggerOrderingGuard(t *testing.T, db *sql.DB) {
	t.Helper()
	var value, marker string
	var auditCount int
	err := db.QueryRow(`
		SELECT value, audit_count, updated_marker
		FROM public.trigger_specimens
		WHERE id = 1
	`).Scan(&value, &auditCount, &marker)
	testutil.NoError(t, err)
	testutil.Equal(t, "source-preserved", value)
	testutil.Equal(t, 77, auditCount)
	testutil.Equal(t, "seeded", marker)
}

func assertPartitionedRowCopiedOnce(t *testing.T, db *sql.DB) {
	t.Helper()
	var count int
	var value string
	err := db.QueryRow(`
		SELECT count(*), COALESCE(min(value), '')
		FROM public.partitioned_trigger_specimens
		WHERE id = 1
	`).Scan(&count, &value)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, count)
	testutil.Equal(t, "partition-source", value)
}

func assertMigratedTriggerSideEffects(t *testing.T, db *sql.DB) {
	t.Helper()
	var insertedValue string
	var insertedAuditCount int
	err := db.QueryRow(`
		INSERT INTO public.trigger_specimens (id, value)
		VALUES (2, 'fresh')
		RETURNING value, audit_count
	`).Scan(&insertedValue, &insertedAuditCount)
	testutil.NoError(t, err)
	testutil.Equal(t, "fresh:inserted", insertedValue)
	testutil.Equal(t, 10, insertedAuditCount)

	var updatedMarker string
	var updatedAuditCount int
	err = db.QueryRow(`
		UPDATE public.trigger_specimens
		SET value = 'fresh-update'
		WHERE id = 2
		RETURNING audit_count, updated_marker
	`).Scan(&updatedAuditCount, &updatedMarker)
	testutil.NoError(t, err)
	testutil.Equal(t, 11, updatedAuditCount)
	testutil.Equal(t, "updated", updatedMarker)
}

func assertDisabledTriggerRemainsInert(t *testing.T, db *sql.DB) {
	t.Helper()
	var value string
	err := db.QueryRow(`
		INSERT INTO public.disabled_trigger_specimens (id, value)
		VALUES (2, 'plain')
		RETURNING value
	`).Scan(&value)
	testutil.NoError(t, err)
	testutil.Equal(t, "plain", value)
	testutil.False(t, strings.Contains(value, "disabled-fired"), "disabled trigger must remain inert")
}

func assertPartitionTriggerClones(t *testing.T, sourceDB, targetDB *sql.DB) {
	t.Helper()
	testutil.Equal(t, 2, countPartitionTriggerClones(t, sourceDB))
	testutil.Equal(t, 2, countPartitionTriggerClones(t, targetDB))
	assertTriggerEnabledParity(t, sourceDB, targetDB, "public", "partitioned_trigger_specimens", "partitioned_before_insert")
	assertTriggerEnabledParity(t, sourceDB, targetDB, "public", "partitioned_trigger_specimens_low", "partitioned_before_insert")
	assertTriggerEnabledParity(t, sourceDB, targetDB, "auth", "partitioned_trigger_specimens_high", "partitioned_before_insert")
	assertTableExists(t, targetDB, "auth", "partitioned_trigger_specimens_high", true)
}

func countPartitionTriggerClones(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	err := db.QueryRow(`
		SELECT count(*)
		FROM pg_trigger
		WHERE tgname = 'partitioned_before_insert'
		  AND tgparentid <> 0
	`).Scan(&count)
	testutil.NoError(t, err)
	return count
}

func assertSkippedPartitionTriggerAbsent(t *testing.T, db *sql.DB) {
	t.Helper()
	assertTriggerAbsent(t, db, "public", "skipped_partitioned_trigger_specimens", "skipped_partitioned_before_insert")
	assertTriggerAbsent(t, db, "public", "skipped_partitioned_trigger_specimens_low", "skipped_partitioned_before_insert")
}

func assertValidationSummaryHasNoRecordMismatch(t *testing.T, report *migrate.AnalysisReport, stats *MigrationStats) {
	t.Helper()
	summary := BuildValidationSummary(report, stats)
	testutil.False(t, strings.Contains(strings.Join(summary.Warnings, "\n"), "Records count mismatch"), "records must validate after partition rows are counted once")
}

func seedOrdinaryInheritanceSource(t *testing.T, sourceDB *sql.DB) {
	t.Helper()
	_, err := sourceDB.Exec(`
		CREATE SCHEMA auth;
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
		CREATE TABLE public.ordinary_parent (
			id integer PRIMARY KEY,
			note text NOT NULL
		);
		CREATE TABLE public.ordinary_child (
			child_marker text NOT NULL
		) INHERITS (public.ordinary_parent);
	`)
	testutil.NoError(t, err)
}

func seedDataSkipTriggerSource(t *testing.T, sourceDB *sql.DB) {
	t.Helper()
	_, err := sourceDB.Exec(`
		CREATE SCHEMA auth;
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
		CREATE TABLE public.data_skip_parents (
			id integer PRIMARY KEY
		);
		CREATE TABLE public.data_skip_children (
			id integer PRIMARY KEY,
			parent_id integer NOT NULL,
			note text NOT NULL
		);
		INSERT INTO public.data_skip_children (id, parent_id, note)
		VALUES (1, 404, 'invalid-source-row');
		ALTER TABLE public.data_skip_children
		ADD CONSTRAINT data_skip_children_parent_id_fkey
		FOREIGN KEY (parent_id) REFERENCES public.data_skip_parents(id) NOT VALID;

		CREATE FUNCTION public.data_skip_trigger_handler()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			NEW.note := NEW.note || ':triggered';
			RETURN NEW;
		END;
		$$;
		CREATE TRIGGER data_skip_children_before_insert
		BEFORE INSERT ON public.data_skip_children
		FOR EACH ROW EXECUTE FUNCTION public.data_skip_trigger_handler();
	`)
	testutil.NoError(t, err)
}

func assertDataSkippedTableTriggerFires(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO public.data_skip_parents (id) VALUES (1)`)
	testutil.NoError(t, err)

	var note string
	err = db.QueryRow(`
		INSERT INTO public.data_skip_children (id, parent_id, note)
		VALUES (2, 1, 'fresh')
		RETURNING note
	`).Scan(&note)
	testutil.NoError(t, err)
	testutil.Equal(t, "fresh:triggered", note)
}
