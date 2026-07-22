//go:build integration

package backup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const restoreRoundTripAmountSum = 24750

func TestPhysicalBackupRestoreRoundTrip(t *testing.T) {
	dbURL := requireTestDBURL(t)
	pgTools := requirePostgresToolchain(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connecting to DB: %v", err)
	}
	defer pool.Close()

	setupTestDB(t, pool)
	setupRestoreRoundTripSchema(t, pool)

	tableName := fmt.Sprintf("restore_roundtrip_%d", time.Now().UnixNano())
	markerTable := tableName + "_markers"
	seedRestoreRoundTripRows(t, ctx, pool, tableName)
	createRestoreRoundTripMarkerTable(t, ctx, pool, markerTable)
	assertRestoreRoundTripRows(t, ctx, pool, tableName)

	archive := requireRoundTripArchiveRuntime(t, ctx, dbURL)
	store := archive.store
	cfg := archive.config
	projectID := archive.projectID
	databaseID := archive.databaseID
	repo := NewPgRepo(pool)
	walRepo := NewPgWALSegmentRepo(pool)
	manifestRepo := NewPgManifestRepo(pool)
	writer := NewManifestWriter(store, manifestRepo, walRepo, cfg)
	runner := NewBaseBackupRunner(dbURL)
	runner.PgBaseBackupPath = pgTools.pgBaseBackupPath
	engine := NewPhysicalEngine(cfg, store, repo, runner, NoopNotifier{}, projectID, databaseID, writer)

	rec, err := repo.CreatePhysical(ctx, projectID, databaseID, "restore-roundtrip-test")
	if err != nil {
		t.Fatalf("creating physical backup record: %v", err)
	}
	if err := engine.RunWithRecord(ctx, rec); err != nil {
		t.Fatalf("physical backup phase failed before restore orchestration: %v", err)
	}

	manifest, err := manifestRepo.GetByBackupID(ctx, rec.ID)
	if err != nil {
		t.Fatalf("loading manifest after physical backup: %v", err)
	}
	if manifest == nil {
		t.Fatalf("physical backup completed without a backup manifest for %s", rec.ID)
	}

	targetTime := recoverableTargetTime(t, ctx, repo, walRepo, store, cfg, rec.ID, projectID, databaseID, markerTable, pool)
	planner := NewRestorePlanner(repo, walRepo, manifestRepo)
	jobRepo := NewPgRestoreJobRepo(pool)
	orchestrator := NewRestoreOrchestrator(planner, jobRepo, store, NoopNotifier{}, cfg, dbURL, cfg.ArchivePrefix, slog.Default())

	job, err := orchestrator.Execute(ctx, projectID, databaseID, targetTime, "restore-roundtrip-test")
	if err != nil {
		t.Fatalf("restore orchestration with default seams failed: %v", err)
	}
	t.Cleanup(func() {
		if abandonErr := orchestrator.Abandon(context.Background(), job.ID); abandonErr != nil {
			t.Logf("abandon restore job %s: %v", job.ID, abandonErr)
		}
	})

	recovery, ok := orchestrator.ActiveInstance(job.ID)
	if !ok {
		t.Fatalf("restore job %s did not leave an active recovery instance", job.ID)
	}
	recoveredPool, err := pgxpool.New(ctx, recovery.ConnURL())
	if err != nil {
		t.Fatalf("connecting to recovery instance: %v", err)
	}
	defer recoveredPool.Close()
	assertRestoreRoundTripRows(t, ctx, recoveredPool, tableName)
	assertRestoreRoundTripMarkers(t, ctx, recoveredPool, markerTable)
}

const (
	restoreRoundTripMarkerBefore = "before-target"
	restoreRoundTripMarkerAfter  = "after-target"
	// How long to wait for the archiver to publish a segment covering the
	// target. Generous enough for a local archive_command, short enough that a
	// missing archiver fails the lane rather than hanging it.
	restoreRoundTripArchiveWait = 60 * time.Second
)

// recoverableTargetTime returns a PITR target that PostgreSQL recovery can
// actually reach, rather than a WAL metadata timestamp.
//
// The target is bracketed by two committed WAL records: the "before-target"
// marker commits strictly before it and the "after-target" marker strictly
// after. Recovery to this timestamp must therefore replay the first marker and
// stop before the second — which is what assertRestoreRoundTripMarkers checks.
// Using the latest segment's archived_at instead would name an instant that no
// commit brackets, so a restore could "succeed" without replaying any WAL.
//
// After bracketing, WAL is forced out with pg_switch_wal and the function waits
// for LSN coverage through the production WAL segment repo. This proves the
// archived chain contains WAL written after the target instead of inferring
// coverage from an archive timestamp. No synthetic WAL rows are ever seeded:
// when archiving never runs, this fails here naming phase=plan.
func recoverableTargetTime(
	t *testing.T,
	ctx context.Context,
	repo Repo,
	walRepo WALSegmentRepo,
	store Store,
	cfg PITRConfig,
	backupID, projectID, databaseID, markerTable string,
	pool *pgxpool.Pool,
) time.Time {
	t.Helper()

	completed, err := repo.Get(ctx, backupID)
	if err != nil {
		t.Fatalf("reloading completed base backup %s: %v", backupID, err)
	}
	if completed == nil || completed.CompletedAt == nil {
		t.Fatalf("base backup %s has no completed_at; cannot bound a recoverable PITR window", backupID)
	}
	if completed.StartLSN == nil || *completed.StartLSN == "" {
		t.Fatalf("base backup %s has no start_lsn; cannot select WAL files for replay", backupID)
	}

	insertRestoreRoundTripMarker(t, ctx, pool, markerTable, restoreRoundTripMarkerBefore)

	// Read the target from the server clock so it is comparable with the
	// commit timestamps PostgreSQL recovery evaluates against recovery_target_time.
	var targetTime time.Time
	if err := pool.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&targetTime); err != nil {
		t.Fatalf("reading server clock for PITR target: %v", err)
	}
	// Keep the bracket unambiguous at one-second timestamp granularity.
	time.Sleep(2 * time.Second)
	insertRestoreRoundTripMarker(t, ctx, pool, markerTable, restoreRoundTripMarkerAfter)
	var replayThroughLSN string
	if err := pool.QueryRow(ctx, "SELECT pg_current_wal_lsn()::text").Scan(&replayThroughLSN); err != nil {
		t.Fatalf("reading WAL position after recovery-target bracket: %v", err)
	}

	if !targetTime.After(*completed.CompletedAt) {
		t.Fatalf("PITR target %s is not after base backup completion %s; the bracket would not require WAL replay",
			targetTime.UTC().Format(time.RFC3339Nano), completed.CompletedAt.UTC().Format(time.RFC3339Nano))
	}

	if _, err := pool.Exec(ctx, "SELECT pg_switch_wal()"); err != nil {
		t.Fatalf("forcing WAL segment switch for automatic archival: %v", err)
	}

	waitForArchivedWALCoveringLSN(t, ctx, walRepo, store, cfg.ArchivePrefix, projectID, databaseID, replayThroughLSN)
	return targetTime.UTC()
}

// waitForArchivedWALCoveringLSN waits until PostgreSQL archives the segment
// containing WAL written after the recovery target. Archive timestamps alone
// cannot prove that a segment contains the target transaction.
func waitForArchivedWALCoveringLSN(
	t *testing.T,
	ctx context.Context,
	walRepo WALSegmentRepo,
	store Store,
	archivePrefix string,
	projectID, databaseID string,
	replayThroughLSN string,
) {
	t.Helper()

	deadline := time.Now().Add(restoreRoundTripArchiveWait)
	for time.Now().Before(deadline) {
		segment, err := walRepo.CoveringSegment(ctx, projectID, databaseID, replayThroughLSN)
		if err != nil {
			t.Fatalf("querying automatically archived WAL coverage for LSN %s: %v", replayThroughLSN, err)
		}
		if segment != nil {
			objectKey := WALSegmentKey(archivePrefix, projectID, databaseID, segment.Timeline, segment.SegmentName)
			objectSize, err := store.HeadObject(ctx, objectKey)
			if err != nil {
				t.Fatalf("automatically archived WAL metadata exists but object %s is missing: %v", objectKey, err)
			}
			if objectSize != segment.SizeBytes {
				t.Fatalf("automatically archived WAL object %s size = %d, metadata size = %d", objectKey, objectSize, segment.SizeBytes)
			}
			t.Logf("WAL segment %s [%s, %s) covers post-target LSN %s",
				segment.SegmentName, segment.StartLSN, segment.EndLSN, replayThroughLSN)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("restore phase=plan: no automatically archived WAL segment exists for project=%s database=%s within %s covering post-target LSN %s; "+
		"continuous WAL archiving never ran, so no committed record can be recovered",
		projectID, databaseID, restoreRoundTripArchiveWait, replayThroughLSN)
}

func requireTestDBURL(t *testing.T) string {
	t.Helper()
	dbURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dbURL == "" {
		t.Fatal("TEST_DATABASE_URL is required for TestPhysicalBackupRestoreRoundTrip")
	}
	return dbURL
}

type postgresToolchain struct {
	binDir           string
	pgBaseBackupPath string
	pgCtlPath        string
}

// requirePostgresToolchain resolves pg_basebackup and pg_ctl from one bin dir
// and guarantees that dir is first on PATH for the restore runtime. The PATH
// guarantee is load-bearing: RecoveryInstance.Start in restore_postgres.go
// executes a bare "pg_ctl" through its default runCommand seam.
func requirePostgresToolchain(t *testing.T) postgresToolchain {
	t.Helper()

	originalPath := os.Getenv("PATH")
	managedBinDir := strings.TrimSpace(os.Getenv("TEST_PG_BIN_DIR"))
	if managedBinDir == "" {
		t.Fatal("TEST_PG_BIN_DIR is required so pg_basebackup and pg_ctl come from the managed Postgres started by testpg")
	}

	toolchain, ok := postgresToolchainFromDir(managedBinDir)
	if !ok {
		t.Fatalf("managed Postgres toolchain at TEST_PG_BIN_DIR=%s does not contain executable pg_basebackup and pg_ctl", managedBinDir)
	}
	t.Setenv("PATH", toolchain.binDir+string(os.PathListSeparator)+originalPath)
	resolvedPgCtl, err := exec.LookPath("pg_ctl")
	if err != nil {
		t.Fatalf("pg_ctl in managed Postgres toolchain became unreachable via PATH: %v", err)
	}
	if resolvedPgCtl != toolchain.pgCtlPath {
		t.Fatalf("pg_ctl resolved to %s after selecting managed toolchain %s; want %s",
			resolvedPgCtl, toolchain.binDir, toolchain.pgCtlPath)
	}
	t.Logf("using managed Postgres toolchain from %s", toolchain.binDir)
	return toolchain

}

func postgresToolchainFromDir(dir string) (postgresToolchain, bool) {
	binDir, err := filepath.Abs(dir)
	if err != nil {
		binDir = dir
	}
	toolchain := postgresToolchain{
		binDir:           binDir,
		pgBaseBackupPath: filepath.Join(binDir, "pg_basebackup"),
		pgCtlPath:        filepath.Join(binDir, "pg_ctl"),
	}
	if !isExecutableFile(toolchain.pgBaseBackupPath) || !isExecutableFile(toolchain.pgCtlPath) {
		return postgresToolchain{}, false
	}
	return toolchain, true
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

func setupRestoreRoundTripSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	statements := []string{
		`CREATE TABLE IF NOT EXISTS _ayb_wal_segments (
			id TEXT NOT NULL DEFAULT gen_random_uuid()::text PRIMARY KEY,
			project_id TEXT NOT NULL,
			database_id TEXT NOT NULL,
			timeline INTEGER NOT NULL,
			segment_name TEXT NOT NULL,
			start_lsn pg_lsn NOT NULL,
			end_lsn pg_lsn NOT NULL,
			checksum TEXT NOT NULL,
			size_bytes BIGINT NOT NULL,
			archived_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT _ayb_wal_segments_unique UNIQUE (project_id, database_id, timeline, segment_name)
		)`,
		`CREATE INDEX IF NOT EXISTS _ayb_wal_segments_project_archived_idx
			ON _ayb_wal_segments (project_id, database_id, archived_at DESC)`,
		`CREATE TABLE IF NOT EXISTS _ayb_backup_manifests (
			id TEXT NOT NULL DEFAULT gen_random_uuid()::text PRIMARY KEY,
			project_id TEXT NOT NULL,
			database_id TEXT NOT NULL,
			backup_id TEXT NOT NULL REFERENCES _ayb_backups (id),
			object_key TEXT NOT NULL,
			start_lsn pg_lsn NOT NULL,
			end_lsn pg_lsn NOT NULL,
			checksum TEXT NOT NULL,
			timeline INTEGER NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS _ayb_backup_manifests_backup_id_key
			ON _ayb_backup_manifests (backup_id)`,
		`CREATE INDEX IF NOT EXISTS _ayb_backup_manifests_project_created_idx
			ON _ayb_backup_manifests (project_id, database_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS _ayb_restore_jobs (
			id TEXT NOT NULL DEFAULT gen_random_uuid()::text PRIMARY KEY,
			project_id TEXT NOT NULL,
			database_id TEXT NOT NULL,
			environment TEXT NOT NULL DEFAULT '',
			target_time TIMESTAMPTZ NOT NULL,
			phase TEXT NOT NULL DEFAULT 'pending',
			status TEXT NOT NULL DEFAULT 'pending',
			base_backup_id TEXT NOT NULL DEFAULT '',
			wal_segments_needed INTEGER NOT NULL DEFAULT 0,
			verification_result JSONB,
			logs TEXT NOT NULL DEFAULT '',
			error_message TEXT NOT NULL DEFAULT '',
			requested_by TEXT NOT NULL DEFAULT '',
			started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			completed_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS _ayb_restore_jobs_project_started_idx
			ON _ayb_restore_jobs (project_id, database_id, started_at DESC)`,
		`DELETE FROM _ayb_restore_jobs`,
		`DELETE FROM _ayb_backup_manifests`,
		`DELETE FROM _ayb_wal_segments`,
	}
	for _, stmt := range statements {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("setup restore roundtrip schema: %v", err)
		}
	}
}

func seedRestoreRoundTripRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tableName string) {
	t.Helper()
	_, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE %s (
			id INTEGER PRIMARY KEY,
			amount_cents INTEGER NOT NULL
		)`, tableName))
	if err != nil {
		t.Fatalf("creating user table: %v", err)
	}
	_, err = pool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s (id, amount_cents) VALUES
			(1, 1010), (2, 2020), (3, 3030), (4, 4040), (5, 5050),
			(6, 6060), (7, 707), (8, 808), (9, 909), (10, 1116)`, tableName))
	if err != nil {
		t.Fatalf("seeding user table: %v", err)
	}
}

// createRestoreRoundTripMarkerTable creates the bracket table before the base
// backup so its rows — inserted afterwards — can only reach the restored
// instance through replayed WAL, never through the base backup itself.
func createRestoreRoundTripMarkerTable(t *testing.T, ctx context.Context, pool *pgxpool.Pool, markerTable string) {
	t.Helper()
	_, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE %s (
			label TEXT PRIMARY KEY,
			committed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
		)`, markerTable))
	if err != nil {
		t.Fatalf("creating PITR marker table: %v", err)
	}
}

func insertRestoreRoundTripMarker(t *testing.T, ctx context.Context, pool *pgxpool.Pool, markerTable, label string) {
	t.Helper()
	if _, err := pool.Exec(ctx, fmt.Sprintf("INSERT INTO %s (label) VALUES ($1)", markerTable), label); err != nil {
		t.Fatalf("committing PITR marker %q: %v", label, err)
	}
}

// assertRestoreRoundTripMarkers proves recovery stopped at the target: the
// marker committed before the target must be present, and the one committed
// after must not be. A restore that replayed no WAL fails the first check; one
// that replayed past the target fails the second.
func assertRestoreRoundTripMarkers(t *testing.T, ctx context.Context, pool *pgxpool.Pool, markerTable string) {
	t.Helper()
	var beforeCount, afterCount int
	query := fmt.Sprintf(
		"SELECT COUNT(*) FILTER (WHERE label = $1), COUNT(*) FILTER (WHERE label = $2) FROM %s", markerTable)
	err := pool.QueryRow(ctx, query, restoreRoundTripMarkerBefore, restoreRoundTripMarkerAfter).
		Scan(&beforeCount, &afterCount)
	if err != nil {
		t.Fatalf("querying PITR markers on the recovered instance: %v", err)
	}
	if beforeCount != 1 {
		t.Fatalf("recovered instance has %d %q markers; want 1 — WAL committed before the target was not replayed",
			beforeCount, restoreRoundTripMarkerBefore)
	}
	if afterCount != 0 {
		t.Fatalf("recovered instance has %d %q markers; want 0 — recovery replayed past the PITR target",
			afterCount, restoreRoundTripMarkerAfter)
	}
}

func assertRestoreRoundTripRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tableName string) {
	t.Helper()
	var count int
	var sum int
	var sentinel int
	query := fmt.Sprintf("SELECT COUNT(*), COALESCE(SUM(amount_cents), 0), MAX(amount_cents) FILTER (WHERE id = 7) FROM %s", tableName)
	if err := pool.QueryRow(ctx, query).Scan(&count, &sum, &sentinel); err != nil {
		t.Fatalf("querying roundtrip rows: %v", err)
	}
	if count != 10 {
		t.Fatalf("row count = %d; want 10", count)
	}
	if sum != restoreRoundTripAmountSum {
		t.Fatalf("amount sum = %d; want %d", sum, restoreRoundTripAmountSum)
	}
	if sentinel != 707 {
		t.Fatalf("sentinel amount for id=7 = %d; want 707", sentinel)
	}
}
