//go:build integration

package backup

import (
	"context"
	"fmt"
	"io"
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

	store := newFileBackupStore(t.TempDir())
	repo := NewPgRepo(pool)
	walRepo := NewPgWALSegmentRepo(pool)
	manifestRepo := NewPgManifestRepo(pool)
	cfg := PITRConfig{
		Enabled:          true,
		ArchiveBucket:    "test-only-filesystem-store",
		ArchivePrefix:    "roundtrip",
		ShadowMode:       false,
		EnvironmentClass: "test",
	}
	writer := NewManifestWriter(store, manifestRepo, walRepo, cfg)
	runner := NewBaseBackupRunner(dbURL)
	runner.PgBaseBackupPath = pgTools.pgBaseBackupPath
	engine := NewPhysicalEngine(cfg, store, repo, runner, NoopNotifier{}, "project-roundtrip", "postgres", writer)

	rec, err := repo.CreatePhysical(ctx, "project-roundtrip", "postgres", "restore-roundtrip-test")
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

	targetTime := recoverableTargetTime(t, ctx, repo, walRepo, store, cfg, rec.ID, "project-roundtrip", "postgres", markerTable, pool)
	planner := NewRestorePlanner(repo, walRepo, manifestRepo)
	jobRepo := NewPgRestoreJobRepo(pool)
	orchestrator := NewRestoreOrchestrator(planner, jobRepo, store, NoopNotifier{}, cfg, dbURL, cfg.ArchivePrefix, slog.Default())

	job, err := orchestrator.Execute(ctx, "project-roundtrip", "postgres", targetTime, "restore-roundtrip-test")
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
// for archival coverage through the production WAL segment repo, so
// RestorePlanner.ValidateWindow's three bounds (target at/after base-backup
// completed_at, at/before latest archived_at, and covered by a segment at or
// after the target) hold by observation, not assumption. No synthetic WAL rows
// are ever seeded: when archiving never runs, this fails here naming
// phase=plan, which is the intended Stage 1 red evidence.
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

	if !targetTime.After(*completed.CompletedAt) {
		t.Fatalf("PITR target %s is not after base backup completion %s; the bracket would not require WAL replay",
			targetTime.UTC().Format(time.RFC3339Nano), completed.CompletedAt.UTC().Format(time.RFC3339Nano))
	}

	switchAndArchiveRoundTripWAL(t, ctx, pool, store, walRepo, cfg, projectID, databaseID, *completed.StartLSN)

	waitForArchivedWALCoveringTarget(t, ctx, walRepo, projectID, databaseID, targetTime)
	return targetTime.UTC()
}

func switchAndArchiveRoundTripWAL(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	store Store,
	walRepo WALSegmentRepo,
	cfg PITRConfig,
	projectID, databaseID, replayStartLSN string,
) {
	t.Helper()

	var walFileName, dataDir string
	if err := pool.QueryRow(ctx, "SELECT pg_walfile_name(pg_current_wal_lsn()), current_setting('data_directory')").
		Scan(&walFileName, &dataDir); err != nil {
		t.Fatalf("locating current WAL file before switch: %v", err)
	}
	if _, err := pool.Exec(ctx, "SELECT pg_switch_wal()"); err != nil {
		t.Fatalf("forcing WAL segment switch: %v", err)
	}

	walFiles := walFilesForReplay(t, filepath.Join(dataDir, "pg_wal"), replayStartLSN, walFileName)
	shipper := NewWALShipper(store, walRepo, cfg, projectID, databaseID, NoopNotifier{})
	for _, walFile := range walFiles {
		waitForWALFile(t, walFile)
		if err := shipper.Ship(ctx, walFile, filepath.Base(walFile)); err != nil {
			t.Fatalf("archiving WAL segment %s: %v", walFile, err)
		}
	}
}

func walFilesForReplay(t *testing.T, walDir, replayStartLSN, targetWALFile string) []string {
	t.Helper()

	replayStart, err := lsnUint64(replayStartLSN)
	if err != nil {
		t.Fatalf("parsing base backup end_lsn %s: %v", replayStartLSN, err)
	}
	target, err := ParseWALFileName(targetWALFile)
	if err != nil {
		t.Fatalf("parsing target WAL file %s: %v", targetWALFile, err)
	}
	targetEnd, err := lsnUint64(target.EndLSN())
	if err != nil {
		t.Fatalf("parsing target WAL end_lsn %s: %v", target.EndLSN(), err)
	}

	matches, err := filepath.Glob(filepath.Join(walDir, "[0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F][0-9A-F]"))
	if err != nil {
		t.Fatalf("listing WAL files in %s: %v", walDir, err)
	}

	var walFiles []string
	for _, path := range matches {
		parsed, err := ParseWALFileName(filepath.Base(path))
		if err != nil {
			continue
		}
		start, err := lsnUint64(parsed.StartLSN())
		if err != nil {
			t.Fatalf("parsing WAL start_lsn for %s: %v", path, err)
		}
		end, err := lsnUint64(parsed.EndLSN())
		if err != nil {
			t.Fatalf("parsing WAL end_lsn for %s: %v", path, err)
		}
		if end > replayStart && start <= targetEnd {
			walFiles = append(walFiles, path)
		}
	}
	if len(walFiles) == 0 {
		t.Fatalf("no WAL files in %s cover replay range from %s through %s", walDir, replayStartLSN, targetWALFile)
	}
	return walFiles
}

func waitForWALFile(t *testing.T, walPath string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		info, err := os.Stat(walPath)
		if err == nil && info.Size() > 0 {
			return
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("WAL file %s did not become readable: %v", walPath, lastErr)
	}
	t.Fatalf("WAL file %s remained empty", walPath)
}

// waitForArchivedWALCoveringTarget polls the production WAL segment repo until
// a segment archived at or after targetTime exists, which is exactly the
// coverage RestorePlanner.ValidateWindow requires.
func waitForArchivedWALCoveringTarget(
	t *testing.T,
	ctx context.Context,
	walRepo WALSegmentRepo,
	projectID, databaseID string,
	targetTime time.Time,
) {
	t.Helper()

	deadline := time.Now().Add(restoreRoundTripArchiveWait)
	var lastSeen *WALSegment
	for time.Now().Before(deadline) {
		latest, err := walRepo.LatestByProject(ctx, projectID, databaseID)
		if err != nil {
			t.Fatalf("querying latest archived WAL segment: %v", err)
		}
		if latest != nil {
			lastSeen = latest
			if !latest.ArchivedAt.Before(targetTime) {
				t.Logf("WAL segment %s archived at %s covers PITR target %s",
					latest.SegmentName,
					latest.ArchivedAt.UTC().Format(time.RFC3339Nano),
					targetTime.UTC().Format(time.RFC3339Nano))
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	if lastSeen == nil {
		t.Fatalf("restore phase=plan: no archived WAL segments exist for project=%s database=%s within %s of the PITR target %s; "+
			"continuous WAL archiving never ran, so no committed record can be recovered",
			projectID, databaseID, restoreRoundTripArchiveWait, targetTime.UTC().Format(time.RFC3339Nano))
	}
	t.Fatalf("restore phase=plan: latest archived WAL segment %s stalled at %s and never reached PITR target %s within %s; "+
		"the target commit is not covered by any archived segment",
		lastSeen.SegmentName, lastSeen.ArchivedAt.UTC().Format(time.RFC3339Nano),
		targetTime.UTC().Format(time.RFC3339Nano), restoreRoundTripArchiveWait)
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

type fileBackupStore struct {
	root string
}

func newFileBackupStore(root string) *fileBackupStore {
	return &fileBackupStore{root: root}
}

func (s *fileBackupStore) PutObject(_ context.Context, key string, body io.Reader, _ int64, _ string) error {
	path, err := s.pathForKey(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, body); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func (s *fileBackupStore) GetObject(_ context.Context, key string) (io.ReadCloser, int64, error) {
	path, err := s.pathForKey(key)
	if err != nil {
		return nil, 0, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, 0, err
	}
	return file, stat.Size(), nil
}

func (s *fileBackupStore) HeadObject(_ context.Context, key string) (int64, error) {
	path, err := s.pathForKey(key)
	if err != nil {
		return 0, err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}

func (s *fileBackupStore) ListObjects(_ context.Context, prefix string) ([]StoreObject, error) {
	var objects []StoreObject
	err := filepath.WalkDir(s.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if !strings.HasPrefix(key, prefix) {
			return nil
		}
		stat, err := d.Info()
		if err != nil {
			return err
		}
		objects = append(objects, StoreObject{Key: key, Size: stat.Size()})
		return nil
	})
	return objects, err
}

func (s *fileBackupStore) DeleteObject(_ context.Context, key string) error {
	path, err := s.pathForKey(key)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (s *fileBackupStore) pathForKey(key string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
		return "", fmt.Errorf("invalid object key %q", key)
	}
	path := filepath.Join(s.root, clean)
	rel, err := filepath.Rel(s.root, path)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid object key %q", key)
	}
	return path, nil
}

var _ Store = (*fileBackupStore)(nil)
