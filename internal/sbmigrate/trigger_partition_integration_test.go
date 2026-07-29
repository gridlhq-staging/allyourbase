//go:build integration

package sbmigrate

import (
	"database/sql"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func seedPreexistingPartitionCloneSource(t *testing.T, sourceDB *sql.DB) {
	t.Helper()
	_, err := sourceDB.Exec(`
		CREATE TABLE public.preexisting_partitioned_trigger_specimens (
			id integer NOT NULL,
			value text NOT NULL
		) PARTITION BY RANGE (id);
		CREATE TABLE auth.preexisting_partitioned_trigger_specimens_high
		PARTITION OF public.preexisting_partitioned_trigger_specimens
		FOR VALUES FROM (100) TO (200);
		CREATE TRIGGER preexisting_partitioned_before_insert
		BEFORE INSERT ON public.preexisting_partitioned_trigger_specimens
		FOR EACH ROW EXECUTE FUNCTION public.apply_trigger_side_effect();
		ALTER TABLE auth.preexisting_partitioned_trigger_specimens_high
		DISABLE TRIGGER preexisting_partitioned_before_insert;
	`)
	testutil.NoError(t, err)
}

func seedPreexistingPartitionCloneTarget(t *testing.T, targetDB *sql.DB) {
	t.Helper()
	_, err := targetDB.Exec(`
		CREATE TABLE public.partitioned_trigger_specimens (
			id integer NOT NULL,
			value text NOT NULL
		) PARTITION BY RANGE (id);
		CREATE TABLE auth.partitioned_trigger_specimens_high
		PARTITION OF public.partitioned_trigger_specimens
		FOR VALUES FROM (100) TO (200);

		CREATE TABLE public.preexisting_partitioned_trigger_specimens (
			id integer NOT NULL,
			value text NOT NULL
		) PARTITION BY RANGE (id);
		CREATE TABLE auth.preexisting_partitioned_trigger_specimens_high
		PARTITION OF public.preexisting_partitioned_trigger_specimens
		FOR VALUES FROM (100) TO (200);
	`)
	testutil.NoError(t, err)
}

func assertPreexistingPartitionTriggerCloneState(t *testing.T, sourceDB, targetDB *sql.DB) {
	t.Helper()
	assertTriggerEnabledParity(
		t,
		sourceDB,
		targetDB,
		"auth",
		"preexisting_partitioned_trigger_specimens_high",
		"preexisting_partitioned_before_insert",
	)
}

func assertExcludedPartitionRowNotCopied(t *testing.T, db *sql.DB) {
	t.Helper()
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*)
		FROM auth.partitioned_trigger_specimens_high
		WHERE id = 101
	`).Scan(&count)
	testutil.NoError(t, err)
	testutil.Equal(t, 0, count)
}
