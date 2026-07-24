package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	minLSNForScan = "0/0"
	maxLSNForScan = "FFFFFFFF/FFFFFFFF"
)

// WALGap represents a discontinuity in archived WAL coverage.
type WALGap struct {
	ExpectedLSN string
	ActualLSN   string
	After       string
}

// WALShipper uploads PostgreSQL archive files to object storage and records
// range metadata for complete WAL segments.
type WALShipper struct {
	store      Store
	walRepo    WALSegmentRepo
	cfg        PITRConfig
	projectID  string
	databaseID string
	notify     Notifier
}

// NewWALShipper constructs a WAL shipper.
func NewWALShipper(store Store, walRepo WALSegmentRepo, cfg PITRConfig, projectID, databaseID string, notify Notifier) *WALShipper {
	if notify == nil {
		notify = NoopNotifier{}
	}
	return &WALShipper{
		store:      store,
		walRepo:    walRepo,
		cfg:        cfg,
		projectID:  projectID,
		databaseID: databaseID,
		notify:     notify,
	}
}

// Ship archives one file supplied by PostgreSQL's archive_command. Complete
// WAL segments receive range metadata; backup and timeline history files do not.
func (w *WALShipper) Ship(ctx context.Context, walFilePath string, walFileName string) error {
	archiveFile, err := parsePostgresArchiveFileName(walFileName)
	if err != nil {
		return fmt.Errorf("parsing PostgreSQL archive filename: %w", err)
	}

	file, err := os.Open(walFilePath)
	if err != nil {
		return fmt.Errorf("opening WAL file %q: %w", walFilePath, err)
	}
	defer file.Close()

	hash := sha256.New()
	sizeBytes, err := io.Copy(hash, file)
	if err != nil {
		return fmt.Errorf("hashing WAL file %q: %w", walFilePath, err)
	}
	checksum := hex.EncodeToString(hash.Sum(nil))

	objectKey := WALSegmentKey(w.cfg.ArchivePrefix, w.projectID, w.databaseID, archiveFile.Timeline, archiveFile.OriginalName)

	if _, err := w.store.HeadObject(ctx, objectKey); err == nil {
		return w.acceptExistingArchiveFile(ctx, archiveFile, objectKey, checksum, sizeBytes)
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewinding WAL file %q: %w", walFilePath, err)
	}
	if err := w.store.PutObject(ctx, objectKey, file, sizeBytes, "application/octet-stream"); err != nil {
		return fmt.Errorf("uploading PostgreSQL archive file %q: %w", archiveFile.OriginalName, err)
	}
	return w.recordSegmentMetadata(ctx, archiveFile, checksum, sizeBytes)
}

func (w *WALShipper) acceptExistingArchiveFile(
	ctx context.Context,
	archiveFile *postgresArchiveFileName,
	objectKey, checksum string,
	sizeBytes int64,
) error {
	if archiveFile.Segment != nil {
		existing, err := w.walRepo.GetByName(ctx, w.projectID, w.databaseID, archiveFile.Timeline, archiveFile.OriginalName)
		if err != nil {
			if isMissingWALMetadataTableError(err) {
				return w.verifyStoredArchiveFile(ctx, objectKey, archiveFile.OriginalName, checksum, sizeBytes)
			}
			return fmt.Errorf("WAL object already exists but metadata lookup failed: %w", err)
		}
		if existing != nil {
			if existing.Checksum != checksum {
				return fmt.Errorf("checksum mismatch for existing WAL segment %q: existing=%s new=%s", archiveFile.OriginalName, existing.Checksum, checksum)
			}
			return nil
		}
	}
	if err := w.verifyStoredArchiveFile(ctx, objectKey, archiveFile.OriginalName, checksum, sizeBytes); err != nil {
		return err
	}
	return w.recordSegmentMetadata(ctx, archiveFile, checksum, sizeBytes)
}

func (w *WALShipper) verifyStoredArchiveFile(
	ctx context.Context,
	objectKey, fileName, expectedChecksum string,
	expectedSize int64,
) error {
	object, sizeBytes, err := w.store.GetObject(ctx, objectKey)
	if err != nil {
		return fmt.Errorf("reading existing PostgreSQL archive file %q: %w", fileName, err)
	}
	defer object.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, object); err != nil {
		return fmt.Errorf("hashing existing PostgreSQL archive file %q: %w", fileName, err)
	}
	checksum := hex.EncodeToString(hash.Sum(nil))
	if sizeBytes != expectedSize || checksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch for existing PostgreSQL archive file %q", fileName)
	}
	return nil
}

func (w *WALShipper) recordSegmentMetadata(
	ctx context.Context,
	archiveFile *postgresArchiveFileName,
	checksum string,
	sizeBytes int64,
) error {
	if archiveFile.Segment == nil {
		return nil
	}
	segment := archiveFile.Segment
	if err := w.walRepo.Record(ctx, WALSegment{
		ProjectID:   w.projectID,
		DatabaseID:  w.databaseID,
		Timeline:    segment.Timeline,
		SegmentName: segment.OriginalName,
		StartLSN:    segment.StartLSN(),
		EndLSN:      segment.EndLSN(),
		Checksum:    checksum,
		SizeBytes:   sizeBytes,
		ArchivedAt:  time.Now().UTC(),
	}); err != nil {
		if isMissingWALMetadataTableError(err) {
			return nil
		}
		return fmt.Errorf("recording WAL segment metadata: %w", err)
	}
	return nil
}

func isMissingWALMetadataTableError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "42P01" &&
		(pgErr.TableName == "_ayb_wal_segments" || strings.Contains(pgErr.Message, "_ayb_wal_segments"))
}

// DetectGaps scans archived WAL metadata for discontinuities.
func (w *WALShipper) DetectGaps(ctx context.Context) ([]WALGap, error) {
	segments, err := w.walRepo.ListRange(ctx, w.projectID, w.databaseID, minLSNForScan, maxLSNForScan)
	if err != nil {
		return nil, fmt.Errorf("listing WAL segments for gap detection: %w", err)
	}

	gaps := make([]WALGap, 0)
	for i := 0; i+1 < len(segments); i++ {
		expected := segments[i].EndLSN
		actual := segments[i+1].StartLSN
		if expected == actual {
			continue
		}
		gaps = append(gaps, WALGap{
			ExpectedLSN: expected,
			ActualLSN:   actual,
			After:       segments[i].SegmentName,
		})
	}

	if len(gaps) > 0 {
		w.notify.OnFailure(ctx, FailureEvent{
			DBName:    w.databaseID,
			Stage:     "wal_gap_detection",
			Err:       fmt.Errorf("detected %d WAL gap(s) for project=%s database=%s", len(gaps), w.projectID, w.databaseID),
			Timestamp: time.Now().UTC(),
		})
	}

	return gaps, nil
}
