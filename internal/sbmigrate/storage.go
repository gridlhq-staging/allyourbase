package sbmigrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/migrate"
	"github.com/allyourbase/ayb/internal/storage"
)

// StorageObject represents a file from Supabase's storage.objects table.
type StorageObject struct {
	ID        string
	BucketID  string
	Name      string
	Size      int64
	MimeType  string
	CreatedAt time.Time
}

// StorageBucket represents a bucket from Supabase's storage.buckets table.
type StorageBucket struct {
	ID     string
	Name   string
	Public bool
}

type storageBucketObjects struct {
	bucket  StorageBucket
	objects []StorageObject
}

type storageDestination struct {
	backend          storage.Backend
	backupScratchDir string
}

type copiedStorageObject struct {
	bucketName   string
	name         string
	size         int64
	contentType  string
	sourceBucket string
	backup       *storageObjectBackup
}

const aybReservedResumableStagingBucket = "ayb_resumable_staging"

// listStorageBuckets queries storage.buckets from the Supabase source database.
func (m *Migrator) listStorageBuckets(ctx context.Context) ([]StorageBucket, error) {
	// Check if storage schema exists.
	var exists bool
	err := m.source.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'storage' AND table_name = 'buckets'
		)
	`).Scan(&exists)
	if err != nil || !exists {
		return nil, nil // no storage schema = no buckets
	}

	hasPublic, err := m.sourceColumnExists(ctx, "storage", "buckets", "public")
	if err != nil {
		return nil, fmt.Errorf("checking storage.buckets.public column: %w", err)
	}

	query := `SELECT id, name, false FROM storage.buckets ORDER BY name`
	if hasPublic {
		query = `SELECT id, name, COALESCE(public, false) FROM storage.buckets ORDER BY name`
	}

	rows, err := m.source.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying storage.buckets: %w", err)
	}
	defer rows.Close()

	var buckets []StorageBucket
	for rows.Next() {
		var b StorageBucket
		if err := rows.Scan(&b.ID, &b.Name, &b.Public); err != nil {
			return nil, fmt.Errorf("scanning bucket: %w", err)
		}
		buckets = append(buckets, b)
	}
	return buckets, rows.Err()
}

// listStorageObjects queries storage.objects from the Supabase source database
// for a given bucket.
func (m *Migrator) listStorageObjects(ctx context.Context, bucketID string) ([]StorageObject, error) {
	rows, err := m.source.QueryContext(ctx, `
		SELECT id, bucket_id, name, COALESCE(metadata->>'size', '0')::bigint,
		       COALESCE(metadata->>'mimetype', 'application/octet-stream'),
		       created_at
		FROM storage.objects
		WHERE bucket_id = $1
		ORDER BY name
	`, bucketID)
	if err != nil {
		return nil, fmt.Errorf("querying storage.objects: %w", err)
	}
	defer rows.Close()

	var objects []StorageObject
	for rows.Next() {
		var o StorageObject
		if err := rows.Scan(&o.ID, &o.BucketID, &o.Name, &o.Size, &o.MimeType, &o.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning storage object: %w", err)
		}
		objects = append(objects, o)
	}
	return objects, rows.Err()
}

func (m *Migrator) migrateStorage(ctx context.Context, phaseIdx, totalPhases int) error {
	phase := migrate.Phase{Name: "Storage files", Index: phaseIdx, Total: totalPhases}

	buckets, err := m.listStorageBuckets(ctx)
	if err != nil {
		return fmt.Errorf("listing buckets: %w", err)
	}

	if len(buckets) == 0 {
		m.completeEmptyStoragePhase(phase)
		return nil
	}

	allBuckets, totalObjects, err := m.loadStorageBucketsWithObjects(ctx, buckets)
	if err != nil {
		return err
	}
	if err := validateStorageBucketNames(allBuckets); err != nil {
		return err
	}
	if err := validateStorageBucketExportPaths(allBuckets, m.opts.StorageExportPath); err != nil {
		return err
	}

	start := m.startStoragePhase(phase, totalObjects)
	destination, err := m.prepareStorageDestinationBackend()
	if err != nil {
		return err
	}

	processed := 0
	for _, bucketObjects := range allBuckets {
		if err := m.registerStorageBucket(ctx, bucketObjects.bucket); err != nil {
			return err
		}
		copiedObjects, err := m.copyStorageBucket(ctx, phase, totalObjects, &processed, destination, bucketObjects)
		if err != nil {
			return err
		}
		for index, copiedObject := range copiedObjects {
			if err := m.registerStorageObject(ctx, copiedObject); err != nil {
				rollbackErr := rollbackCopiedStorageObjects(
					context.WithoutCancel(ctx),
					destination.backend,
					copiedObjects[index:],
				)
				return errors.Join(err, rollbackErr)
			}
			copiedObject.backup.discard()
		}
	}

	m.progress.CompletePhase(phase, totalObjects, time.Since(start))
	fmt.Fprintf(m.output, "  %d files migrated (%s)\n",
		m.stats.StorageFiles, migrate.FormatBytes(m.stats.StorageBytes))
	return nil
}

func (m *Migrator) completeEmptyStoragePhase(phase migrate.Phase) {
	m.progress.StartPhase(phase, 0)
	m.progress.CompletePhase(phase, 0, 0)
	fmt.Fprintln(m.output, "No storage buckets found (skipping)")
}

func (m *Migrator) loadStorageBucketsWithObjects(
	ctx context.Context,
	buckets []StorageBucket,
) ([]storageBucketObjects, int, error) {
	allBuckets := make([]storageBucketObjects, 0, len(buckets))
	totalObjects := 0

	for _, bucket := range buckets {
		objects, err := m.listStorageObjects(ctx, bucket.ID)
		if err != nil {
			return nil, 0, fmt.Errorf("listing objects in bucket %s: %w", bucket.Name, err)
		}
		allBuckets = append(allBuckets, storageBucketObjects{bucket: bucket, objects: objects})
		totalObjects += len(objects)
	}

	return allBuckets, totalObjects, nil
}

func validateStorageBucketNames(buckets []storageBucketObjects) error {
	sourceByNormalizedName := make(map[string]string, len(buckets))
	for _, bucketObjects := range buckets {
		sourceName := bucketObjects.bucket.Name
		normalizedName := normalizeBucketName(sourceName)
		if normalizedName == aybReservedResumableStagingBucket {
			return fmt.Errorf(
				"storage bucket %q normalizes to reserved AYB bucket name %q",
				sourceName,
				normalizedName,
			)
		}
		if previousSourceName, ok := sourceByNormalizedName[normalizedName]; ok && previousSourceName != sourceName {
			return fmt.Errorf(
				"storage bucket names %q and %q both normalize to %q",
				previousSourceName,
				sourceName,
				normalizedName,
			)
		}
		sourceByNormalizedName[normalizedName] = sourceName
	}
	return nil
}

func validateStorageBucketExportPaths(buckets []storageBucketObjects, exportRoot string) error {
	for _, bucketObjects := range buckets {
		sourceName := bucketObjects.bucket.Name
		cleanSourceName := filepath.Clean(sourceName)
		exportBucketDir := filepath.Join(exportRoot, sourceName)
		if sourceName == "" ||
			cleanSourceName == "." ||
			filepath.IsAbs(sourceName) ||
			cleanSourceName != sourceName ||
			!isStoragePathWithinRoot(exportBucketDir, exportRoot) {
			return fmt.Errorf(
				"storage bucket %q has unsafe export directory under %q",
				sourceName,
				exportRoot,
			)
		}
	}
	return nil
}

func (m *Migrator) startStoragePhase(phase migrate.Phase, totalObjects int) time.Time {
	m.progress.StartPhase(phase, totalObjects)
	fmt.Fprintln(m.output, "Migrating storage files...")
	return time.Now()
}

func (m *Migrator) prepareStorageDestinationBackend() (storageDestination, error) {
	destPath := m.opts.StoragePath
	if destPath == "" {
		destPath = filepath.Join(".", "ayb_storage")
	}

	absDestPath, err := filepath.Abs(destPath)
	if err != nil {
		return storageDestination{}, fmt.Errorf("resolving storage path: %w", err)
	}

	backend, err := storage.NewLocalBackend(absDestPath)
	if err != nil {
		return storageDestination{}, fmt.Errorf("creating storage backend: %w", err)
	}
	return storageDestination{
		backend:          backend,
		backupScratchDir: filepath.Join(absDestPath, ".ayb-migration-backups"),
	}, nil
}

func (m *Migrator) registerStorageBucket(ctx context.Context, bucket StorageBucket) error {
	normalizedName := normalizeBucketName(bucket.Name)
	_, err := m.target.ExecContext(ctx, `
		INSERT INTO _ayb_storage_buckets (tenant_id, name, public)
		VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, name) DO UPDATE
		SET public = EXCLUDED.public, updated_at = NOW()
	`, "", normalizedName, bucket.Public)
	if err != nil {
		return fmt.Errorf("recording storage bucket %s metadata: %w", bucket.Name, err)
	}
	return nil
}

func (m *Migrator) registerStorageObject(ctx context.Context, obj copiedStorageObject) error {
	_, err := m.target.ExecContext(ctx, `
		INSERT INTO _ayb_storage_objects (tenant_id, bucket, name, size, content_type, user_id)
		VALUES ($1, $2, $3, $4, $5, NULL)
		ON CONFLICT (tenant_id, bucket, name) DO UPDATE
		SET size = EXCLUDED.size, content_type = EXCLUDED.content_type, updated_at = NOW()
	`, "", obj.bucketName, obj.name, obj.size, obj.contentType)
	if err != nil {
		return fmt.Errorf("recording storage object %s/%s metadata: %w", obj.sourceBucket, obj.name, err)
	}
	return nil
}

func (m *Migrator) copyStorageBucket(
	ctx context.Context,
	phase migrate.Phase,
	totalObjects int,
	processed *int,
	destination storageDestination,
	bucketObjects storageBucketObjects,
) ([]copiedStorageObject, error) {
	bucket := bucketObjects.bucket
	objects := bucketObjects.objects
	if len(objects) == 0 {
		if m.verbose {
			fmt.Fprintf(m.output, "  %s: 0 files\n", bucket.Name)
		}
		return nil, nil
	}

	bucketName := normalizeBucketName(bucket.Name)
	copied := 0
	copiedObjects := make([]copiedStorageObject, 0, len(objects))
	for _, obj := range objects {
		exportBucketDir := filepath.Join(m.opts.StorageExportPath, bucket.Name)
		srcFile := filepath.Join(exportBucketDir, obj.Name)
		if !isStoragePathWithinRoot(exportBucketDir, m.opts.StorageExportPath) ||
			!isStoragePathWithinRoot(srcFile, exportBucketDir) {
			m.recordStorageObjectError(phase, processed, totalObjects,
				fmt.Sprintf("skipping %s/%s: path traversal detected", bucket.Name, obj.Name))
			continue
		}

		backup, err := backupStorageObject(ctx, destination.backend, destination.backupScratchDir, bucketName, obj.Name)
		if err != nil {
			m.recordStorageObjectError(phase, processed, totalObjects,
				fmt.Sprintf("preserving %s/%s before replacement: %v", bucket.Name, obj.Name, err))
			continue
		}
		bytes, err := copyFile(ctx, destination.backend, bucketName, obj.Name, srcFile)
		if err != nil {
			backup.discard()
			m.recordStorageObjectError(phase, processed, totalObjects,
				fmt.Sprintf("copying %s/%s: %v", bucket.Name, obj.Name, err))
			continue
		}

		copied++
		copiedObjects = append(copiedObjects, copiedStorageObject{
			bucketName:   bucketName,
			name:         obj.Name,
			size:         bytes,
			contentType:  obj.MimeType,
			sourceBucket: bucket.Name,
			backup:       backup,
		})
		m.stats.StorageFiles++
		m.stats.StorageBytes += bytes
		(*processed)++
		m.progress.Progress(phase, *processed, totalObjects)
	}

	if m.verbose {
		fmt.Fprintf(m.output, "  %s: %d files copied\n", bucket.Name, copied)
	}
	return copiedObjects, nil
}

func isStoragePathWithinRoot(path, root string) bool {
	cleanRoot := filepath.Clean(root)
	cleanPath := filepath.Clean(path)
	rootForRel, err := filepath.Abs(cleanRoot)
	if err != nil {
		return false
	}
	pathForRel, err := filepath.Abs(cleanPath)
	if err != nil {
		return false
	}

	if _, err := os.Lstat(cleanPath); err == nil {
		pathForRel, err = filepath.EvalSymlinks(cleanPath)
		if err != nil {
			return false
		}
		if _, err := os.Lstat(cleanRoot); err == nil {
			rootForRel, err = filepath.EvalSymlinks(cleanRoot)
			if err != nil {
				return false
			}
		}
	} else if !os.IsNotExist(err) {
		return false
	}

	relativePath, err := filepath.Rel(rootForRel, pathForRel)
	if err != nil {
		return false
	}
	return relativePath != ".." &&
		!strings.HasPrefix(relativePath, ".."+string(filepath.Separator))
}

func (m *Migrator) recordStorageObjectError(
	phase migrate.Phase,
	processed *int,
	totalObjects int,
	message string,
) {
	m.stats.Errors = append(m.stats.Errors, message)
	(*processed)++
	m.progress.Progress(phase, *processed, totalObjects)
}

// copyFile copies a source export through AYB's canonical storage backend.
func copyFile(
	ctx context.Context,
	backend storage.Backend,
	bucketName string,
	objectName string,
	src string,
) (int64, error) {
	sf, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("opening source: %w", err)
	}
	defer sf.Close()

	n, err := backend.Put(ctx, "", bucketName, objectName, sf)
	if err != nil {
		return 0, fmt.Errorf("copying data: %w", err)
	}
	return n, nil
}

// normalizeBucketName converts a Supabase bucket name to an AYB-compatible name.
// AYB bucket names: lowercase, letters/digits/hyphens/underscores, max 63 chars.
func normalizeBucketName(name string) string {
	name = strings.ToLower(name)
	var sb strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			sb.WriteRune(c)
		} else if c == ' ' || c == '.' {
			sb.WriteRune('-')
		}
	}
	result := sb.String()
	if len(result) > 63 {
		result = result[:63]
	}
	if result == "" {
		result = "default"
	}
	return result
}
