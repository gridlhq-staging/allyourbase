package sbmigrate

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/storage"
	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNormalizeBucketName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"lowercase passthrough", "avatars", "avatars"},
		{"uppercase converted", "Avatars", "avatars"},
		{"mixed case", "My-Bucket_123", "my-bucket_123"},
		{"spaces to hyphens", "my bucket", "my-bucket"},
		{"dots to hyphens", "my.bucket", "my-bucket"},
		{"special chars stripped", "my@bucket!", "mybucket"},
		{"empty becomes default", "", "default"},
		{"only special chars", "@#$%", "default"},
		{"long name truncated", strings.Repeat("a", 100), strings.Repeat("a", 63)},
		{"digits preserved", "bucket123", "bucket123"},
		{"hyphens preserved", "my-bucket", "my-bucket"},
		{"underscores preserved", "my_bucket", "my_bucket"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeBucketName(tt.input)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestValidateStorageBucketNamesRejectsNormalizedCollisions(t *testing.T) {
	t.Parallel()

	buckets := []storageBucketObjects{
		{bucket: StorageBucket{Name: "Team Docs"}},
		{bucket: StorageBucket{Name: "Team.Docs"}},
	}

	err := validateStorageBucketNames(buckets)

	testutil.ErrorContains(t, err, "Team Docs")
	testutil.ErrorContains(t, err, "Team.Docs")
	testutil.ErrorContains(t, err, "team-docs")
}

func TestListStorageBucketsPropagatesSchemaProbeErrors(t *testing.T) {
	t.Parallel()

	source, err := sql.Open("pgx", "postgres://unused")
	testutil.NoError(t, err)
	testutil.NoError(t, source.Close())

	m := &Migrator{source: source}
	_, err = m.listStorageBuckets(context.Background())
	testutil.ErrorContains(t, err, "checking storage.buckets existence")
	testutil.ErrorContains(t, err, "database is closed")
}

func TestIsStoragePathWithinRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		root string
		want bool
	}{
		{name: "current directory child", path: "bucket", root: ".", want: true},
		{name: "filesystem root child", path: filepath.Join(string(filepath.Separator), "bucket"), root: string(filepath.Separator), want: true},
		{name: "parent escape", path: filepath.Join("..", "bucket"), root: ".", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			testutil.Equal(t, tt.want, isStoragePathWithinRoot(tt.path, tt.root))
		})
	}
}

func TestCopyFile(t *testing.T) {
	t.Parallel()
	t.Run("copies file contents", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		srcPath := filepath.Join(dir, "source.txt")
		storageRoot := filepath.Join(dir, "storage")
		backend, err := storage.NewLocalBackend(storageRoot)
		testutil.NoError(t, err)

		content := []byte("hello world")
		testutil.NoError(t, os.WriteFile(srcPath, content, 0644))

		n, err := copyFile(context.Background(), backend, "test", "dest.txt", srcPath)
		testutil.NoError(t, err)
		testutil.Equal(t, int64(len(content)), n)

		got, err := os.ReadFile(filepath.Join(storageRoot, "test", "dest.txt"))
		testutil.NoError(t, err)
		testutil.Equal(t, string(content), string(got))
	})

	t.Run("copies binary data", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		srcPath := filepath.Join(dir, "binary.bin")
		storageRoot := filepath.Join(dir, "storage")
		backend, err := storage.NewLocalBackend(storageRoot)
		testutil.NoError(t, err)

		// Write binary data with null bytes.
		data := []byte{0x00, 0x01, 0xFF, 0xFE, 0x00, 0x80}
		testutil.NoError(t, os.WriteFile(srcPath, data, 0644))

		n, err := copyFile(context.Background(), backend, "test", "copy.bin", srcPath)
		testutil.NoError(t, err)
		testutil.Equal(t, int64(len(data)), n)

		got, err := os.ReadFile(filepath.Join(storageRoot, "test", "copy.bin"))
		testutil.NoError(t, err)
		testutil.SliceLen(t, got, len(data))
		testutil.True(t, string(data) == string(got), "binary content mismatch")
	})

	t.Run("source not found", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		backend, err := storage.NewLocalBackend(filepath.Join(dir, "storage"))
		testutil.NoError(t, err)
		_, err = copyFile(context.Background(), backend, "test", "dest", filepath.Join(dir, "nonexistent"))
		testutil.ErrorContains(t, err, "opening source")
	})

	t.Run("invalid object name", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		srcPath := filepath.Join(dir, "source.txt")
		storageRoot := filepath.Join(dir, "storage")
		backend, err := storage.NewLocalBackend(storageRoot)
		testutil.NoError(t, err)
		testutil.NoError(t, os.WriteFile(srcPath, []byte("data"), 0644))

		_, err = copyFile(context.Background(), backend, "test", "report..final.txt", srcPath)
		testutil.ErrorContains(t, err, "invalid object name")
		_, statErr := os.Stat(filepath.Join(storageRoot, "test", "report..final.txt"))
		testutil.True(t, os.IsNotExist(statErr), "invalid object was copied: %v", statErr)
	})
}

func TestRollbackCopiedStorageObjectsRestoresExistingAndDeletesNew(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	storageRoot := t.TempDir()
	backend, err := storage.NewLocalBackend(storageRoot)
	testutil.NoError(t, err)

	_, err = backend.Put(ctx, "", "uploads", "existing.txt", strings.NewReader("original"))
	testutil.NoError(t, err)
	scratchDir := filepath.Join(storageRoot, ".ayb-migration-backups")
	existingBackup, err := backupStorageObject(ctx, backend, scratchDir, "uploads", "existing.txt")
	testutil.NoError(t, err)
	newBackup, err := backupStorageObject(ctx, backend, scratchDir, "uploads", "new.txt")
	testutil.NoError(t, err)

	_, err = backend.Put(ctx, "", "uploads", "existing.txt", strings.NewReader("replacement"))
	testutil.NoError(t, err)
	_, err = backend.Put(ctx, "", "uploads", "new.txt", strings.NewReader("new"))
	testutil.NoError(t, err)

	err = rollbackCopiedStorageObjects(ctx, backend, []copiedStorageObject{
		{bucketName: "uploads", name: "existing.txt", backup: existingBackup},
		{bucketName: "uploads", name: "new.txt", backup: newBackup},
	})
	testutil.NoError(t, err)

	reader, err := backend.Get(ctx, "", "uploads", "existing.txt")
	testutil.NoError(t, err)
	defer reader.Close()
	got, err := io.ReadAll(reader)
	testutil.NoError(t, err)
	testutil.True(t, bytes.Equal([]byte("original"), got), "restored bytes = %q", got)

	exists, err := backend.Exists(ctx, "", "uploads", "new.txt")
	testutil.NoError(t, err)
	testutil.False(t, exists, "new object remained after metadata rollback")
}

func TestBackupStorageObjectUsesStorageRootScratch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	storageRoot := t.TempDir()
	backend, err := storage.NewLocalBackend(storageRoot)
	testutil.NoError(t, err)

	_, err = backend.Put(ctx, "", "uploads", "existing.txt", strings.NewReader("original"))
	testutil.NoError(t, err)

	backup, err := backupStorageObject(ctx, backend, filepath.Join(storageRoot, ".ayb-migration-backups"), "uploads", "existing.txt")
	testutil.NoError(t, err)
	t.Cleanup(backup.discard)

	testutil.True(t,
		isStoragePathWithinRoot(backup.path, storageRoot),
		"backup path %q is outside storage root %q",
		backup.path,
		storageRoot,
	)
}

func TestPrintStatsWithStorage(t *testing.T) {
	t.Parallel()
	var buf strings.Builder
	m := &Migrator{
		output: &buf,
		stats: MigrationStats{
			Users:        10,
			StorageFiles: 25,
			StorageBytes: 5 * 1024 * 1024,
		},
	}
	m.printStats()
	out := buf.String()
	testutil.Contains(t, out, "Files:      25 (5.0 MB)")
	testutil.Contains(t, out, "Users:      10")
}

func TestPrintStatsNoStorage(t *testing.T) {
	t.Parallel()
	var buf strings.Builder
	m := &Migrator{
		output: &buf,
		stats:  MigrationStats{Users: 10},
	}
	m.printStats()
	out := buf.String()
	testutil.False(t, strings.Contains(out, "Files:"), "should not show Files when zero")
}

func TestBuildValidationSummaryWithStorage(t *testing.T) {
	t.Parallel()
	report := &migrate.AnalysisReport{
		AuthUsers: 10,
		Files:     25,
	}
	stats := &MigrationStats{
		Users:        10,
		StorageFiles: 25,
	}
	summary := BuildValidationSummary(report, stats)

	// Find storage row.
	var found bool
	for _, row := range summary.Rows {
		if row.Label == "Storage files" {
			found = true
			testutil.Equal(t, 25, row.SourceCount)
			testutil.Equal(t, 25, row.TargetCount)
		}
	}
	testutil.True(t, found, "should have Storage files row in validation summary")
}

func TestBuildValidationSummaryNoStorage(t *testing.T) {
	t.Parallel()
	report := &migrate.AnalysisReport{AuthUsers: 10}
	stats := &MigrationStats{Users: 10}
	summary := BuildValidationSummary(report, stats)

	for _, row := range summary.Rows {
		testutil.True(t, row.Label != "Storage files",
			"should not have Storage files row when counts are zero")
	}
}

func TestMigrateStorageCompleteEmptyStoragePhase(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	m := &Migrator{
		output:   &out,
		progress: migrate.NopReporter{},
	}

	m.completeEmptyStoragePhase(migrate.Phase{Name: "Storage files", Index: 6, Total: 6})

	testutil.Contains(t, out.String(), "No storage buckets found (skipping)")
}

func TestMigrateStorageCopyStorageBucket(t *testing.T) {
	t.Parallel()

	t.Run("empty bucket short-circuits when verbose", func(t *testing.T) {
		t.Parallel()

		var out strings.Builder
		m := &Migrator{
			output:   &out,
			verbose:  true,
			progress: migrate.NopReporter{},
		}

		processed := 0
		backend, backendErr := storage.NewLocalBackend(t.TempDir())
		testutil.NoError(t, backendErr)
		copiedObjects, err := m.copyStorageBucket(
			context.Background(),
			migrate.Phase{Name: "Storage files", Index: 1, Total: 1},
			0,
			&processed,
			localStorageExportSource{root: t.TempDir()},
			storageDestination{
				backend:          backend,
				backupScratchDir: filepath.Join(t.TempDir(), ".ayb-migration-backups"),
			},
			storageBucketObjects{
				bucket:  StorageBucket{Name: "avatars"},
				objects: nil,
			},
		)
		testutil.NoError(t, err)
		testutil.SliceLen(t, copiedObjects, 0)
		testutil.Equal(t, 0, processed)
		testutil.Equal(t, 0, m.stats.StorageFiles)
		testutil.Equal(t, int64(0), m.stats.StorageBytes)
		testutil.Contains(t, out.String(), "avatars: 0 files")
	})

	t.Run("continues after per-object errors and accumulates stats", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		exportPath := filepath.Join(root, "storage-export")
		destPath := filepath.Join(root, "ayb-storage")
		validContent := []byte("photo-content")
		bucket := StorageBucket{Name: "Uploads Assets"}

		srcValidPath := filepath.Join(exportPath, bucket.Name, "images", "photo.jpg")
		testutil.NoError(t, os.MkdirAll(filepath.Dir(srcValidPath), 0755))
		testutil.NoError(t, os.WriteFile(srcValidPath, validContent, 0644))

		var out strings.Builder
		m := &Migrator{
			output:   &out,
			verbose:  true,
			progress: migrate.NopReporter{},
			opts: MigrationOptions{
				StorageExportPath: exportPath,
			},
		}

		objects := []StorageObject{
			{Name: "images/photo.jpg"},
			{Name: "../outside.txt"},
			{Name: "missing.bin"},
		}

		processed := 0
		backend, backendErr := storage.NewLocalBackend(destPath)
		testutil.NoError(t, backendErr)
		copiedObjects, err := m.copyStorageBucket(
			context.Background(),
			migrate.Phase{Name: "Storage files", Index: 1, Total: 1},
			len(objects),
			&processed,
			localStorageExportSource{root: exportPath},
			storageDestination{
				backend:          backend,
				backupScratchDir: filepath.Join(destPath, ".ayb-migration-backups"),
			},
			storageBucketObjects{bucket: bucket, objects: objects},
		)
		testutil.NoError(t, err)

		testutil.SliceLen(t, copiedObjects, 1)
		testutil.Equal(t, normalizeBucketName(bucket.Name), copiedObjects[0].bucketName)
		testutil.Equal(t, "images/photo.jpg", copiedObjects[0].name)
		testutil.Equal(t, int64(len(validContent)), copiedObjects[0].size)
		testutil.Equal(t, "", copiedObjects[0].contentType)
		testutil.Equal(t, bucket.Name, copiedObjects[0].sourceBucket)
		testutil.Equal(t, 3, processed)
		testutil.Equal(t, 1, m.stats.StorageFiles)
		testutil.Equal(t, int64(len(validContent)), m.stats.StorageBytes)
		testutil.SliceLen(t, m.stats.Errors, 2)
		testutil.Contains(t, strings.Join(m.stats.Errors, "\n"), "path traversal detected")
		testutil.Contains(t, strings.Join(m.stats.Errors, "\n"), "opening Uploads Assets/missing.bin")

		destFile := filepath.Join(destPath, normalizeBucketName(bucket.Name), "images", "photo.jpg")
		got, readErr := os.ReadFile(destFile)
		testutil.NoError(t, readErr)
		testutil.Equal(t, string(validContent), string(got))
		testutil.Contains(t, out.String(), "Uploads Assets: 1 files copied")
	})

	t.Run("rejects symlink escapes from export root", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		exportPath := filepath.Join(root, "storage-export")
		destPath := filepath.Join(root, "ayb-storage")
		outsideDir := filepath.Join(root, "outside")
		outsideFile := filepath.Join(outsideDir, "secret.txt")
		bucket := StorageBucket{Name: "Uploads"}

		testutil.NoError(t, os.MkdirAll(filepath.Join(exportPath, bucket.Name), 0755))
		testutil.NoError(t, os.MkdirAll(outsideDir, 0755))
		testutil.NoError(t, os.WriteFile(outsideFile, []byte("secret"), 0644))
		testutil.NoError(t, os.Symlink(outsideFile, filepath.Join(exportPath, bucket.Name, "linked.txt")))

		m := &Migrator{
			progress: migrate.NopReporter{},
			opts: MigrationOptions{
				StorageExportPath: exportPath,
			},
		}

		processed := 0
		backend, backendErr := storage.NewLocalBackend(destPath)
		testutil.NoError(t, backendErr)
		copiedObjects, err := m.copyStorageBucket(
			context.Background(),
			migrate.Phase{Name: "Storage files", Index: 1, Total: 1},
			1,
			&processed,
			localStorageExportSource{root: exportPath},
			storageDestination{
				backend:          backend,
				backupScratchDir: filepath.Join(destPath, ".ayb-migration-backups"),
			},
			storageBucketObjects{
				bucket:  bucket,
				objects: []StorageObject{{Name: "linked.txt"}},
			},
		)
		testutil.NoError(t, err)

		testutil.SliceLen(t, copiedObjects, 0)
		testutil.Equal(t, 1, processed)
		testutil.Equal(t, 0, m.stats.StorageFiles)
		testutil.SliceLen(t, m.stats.Errors, 1)
		testutil.Contains(t, m.stats.Errors[0], "path traversal detected")
		_, statErr := os.Stat(filepath.Join(destPath, "uploads", "linked.txt"))
		testutil.True(t, os.IsNotExist(statErr), "symlinked outside file was copied: %v", statErr)
	})
}
