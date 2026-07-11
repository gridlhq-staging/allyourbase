// Package storage resumable.go implements resumable upload sessions using the TUS 1.0.0 protocol, with database-backed session management and backend-backed chunk staging. It handles session lifecycle, finalization to backend storage, and cleanup of expired uploads with RLS support. The staging byte mechanics live in resumable_staging.go.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/jackc/pgx/v5"
)

// Decision: we implement the TUS 1.0.0 core protocol directly in this package.
// This keeps the implementation small, avoids adding a large tusd dependency, and
// preserves first-class ownership/RLS integration via the existing Service API.

const (
	resumableUploadStatusActive     = "active"
	resumableUploadStatusFinalizing = "finalizing"
)

const resumableUploadTTL = 24 * time.Hour

var (
	ErrResumableUploadNotFound       = errors.New("resumable upload not found")
	ErrResumableUploadOffsetMismatch = errors.New("resumable upload offset mismatch")
	ErrResumableUploadExpired        = errors.New("resumable upload has expired")
	ErrResumableUploadChunkTooLarge  = errors.New("resumable upload chunk exceeds declared size")
	ErrResumableUploadInvalidState   = errors.New("resumable upload is not in an active state")
	ErrResumableUploadForbidden      = errors.New("resumable upload forbidden: not the upload owner")
)

// enforceUploadOwnership checks that the caller (identified by callerUserID) is
// the owner of the upload. A nil callerUserID (admin) bypasses the check.
// Ownerless uploads (UserID is nil) are reserved for admin-only access.
func enforceUploadOwnership(upload *ResumableUpload, callerUserID *string) error {
	if callerUserID == nil {
		return nil // admin bypass
	}
	if upload.UserID == nil {
		return ErrResumableUploadForbidden
	}
	if *upload.UserID != *callerUserID {
		return ErrResumableUploadForbidden
	}
	return nil
}

// ResumableUpload describes resumable upload session state stored in
// _ayb_storage_uploads.
type ResumableUpload struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"-"`
	Bucket       string    `json:"bucket"`
	Name         string    `json:"name"`
	Path         string    `json:"-"`
	ContentType  string    `json:"contentType"`
	UserID       *string   `json:"userId,omitempty"`
	TotalSize    int64     `json:"totalSize"`
	UploadedSize int64     `json:"uploadedSize"`
	Status       string    `json:"status"`
	ExpiresAt    time.Time `json:"expiresAt"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type resumableRow interface {
	Scan(dest ...any) error
}

type resumableQueryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Resumable session IDs are looked up with tenant_id so another tenant cannot
// append to or finalize a same-ID session if an ID leaks.
const resumableUploadSelectQuery = `
SELECT id, tenant_id, bucket, name, path, content_type, user_id,
       total_size, uploaded_size, status, expires_at, created_at, updated_at
  FROM _ayb_storage_uploads
 WHERE id = $1 AND tenant_id = $2`

func scanResumableUpload(row resumableRow) (*ResumableUpload, error) {
	var upload ResumableUpload
	if err := row.Scan(
		&upload.ID, &upload.TenantID, &upload.Bucket, &upload.Name, &upload.Path, &upload.ContentType,
		&upload.UserID, &upload.TotalSize, &upload.UploadedSize, &upload.Status,
		&upload.ExpiresAt, &upload.CreatedAt, &upload.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrResumableUploadNotFound
		}
		return nil, fmt.Errorf("querying resumable upload: %w", err)
	}
	return &upload, nil
}

func getResumableUpload(ctx context.Context, q resumableQueryer, id, tenantID string, lock bool) (*ResumableUpload, error) {
	query := resumableUploadSelectQuery
	if lock {
		query = query + " FOR UPDATE"
	}
	row := q.QueryRow(ctx, query, id, tenantID)
	return scanResumableUpload(row)
}

// Resumable cleanup needs tx-scoped quota reclamation so the upload row delete
// can roll back if usage accounting fails.
func decrementUsageInTx(ctx context.Context, tx pgx.Tx, tenantID, userID string, bytes int64) error {
	if userID == "" || bytes <= 0 {
		return nil
	}

	_, err := tx.Exec(ctx,
		`UPDATE _ayb_storage_usage
		 SET bytes_used = GREATEST(bytes_used - $3, 0), updated_at = NOW()
		 WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID, bytes,
	)
	if err != nil {
		return fmt.Errorf("decrementing storage usage: %w", err)
	}
	return nil
}

// CreateResumableUpload creates a new resumable session record and returns it.
func (s *Service) CreateResumableUpload(ctx context.Context, bucket, name, contentType string, userID *string, totalSize int64) (*ResumableUpload, error) {
	if err := validateBucket(bucket); err != nil {
		return nil, err
	}
	if err := validateName(name); err != nil {
		return nil, err
	}
	if totalSize <= 0 {
		return nil, fmt.Errorf("upload length must be greater than 0")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if s.pool == nil {
		return nil, fmt.Errorf("database pool is not configured")
	}
	tenantID := tenant.TenantFromContext(ctx)

	// The staging token doubles as the backend object key for this upload's
	// in-progress bytes (_ayb_storage_uploads.path). No backend object is written
	// yet; an absent object is the empty-upload state until the first append.
	stagingToken, err := newStagingToken()
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().Add(resumableUploadTTL)

	// Store tenant_id on the session so later append/finalize paths can enforce
	// the same tenant boundary and write the final object into the right namespace.
	query := `
		INSERT INTO _ayb_storage_uploads
		(tenant_id, bucket, name, path, content_type, user_id, total_size, uploaded_size, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 0, $8, $9)
		RETURNING id, tenant_id, bucket, name, path, content_type, user_id, total_size,
		          uploaded_size, status, expires_at, created_at, updated_at`

	var upload ResumableUpload
	err = s.pool.QueryRow(
		ctx, query,
		tenantID, bucket, name, stagingToken, contentType, userID, totalSize,
		resumableUploadStatusActive, expiresAt,
	).Scan(
		&upload.ID, &upload.TenantID, &upload.Bucket, &upload.Name, &upload.Path, &upload.ContentType, &upload.UserID,
		&upload.TotalSize, &upload.UploadedSize, &upload.Status, &upload.ExpiresAt,
		&upload.CreatedAt, &upload.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating resumable upload: %w", err)
	}

	return &upload, nil
}

// GetResumableUpload returns resumable session metadata by ID.
// callerUserID enforces ownership: non-nil means the caller must own the upload,
// nil means admin bypass. Returns ErrResumableUploadExpired if the session has expired.
func (s *Service) GetResumableUpload(ctx context.Context, id string, callerUserID *string) (*ResumableUpload, error) {
	if id == "" {
		return nil, ErrResumableUploadNotFound
	}
	if s.pool == nil {
		return nil, fmt.Errorf("database pool is not configured")
	}
	upload, err := getResumableUpload(ctx, s.pool, id, tenant.TenantFromContext(ctx), false)
	if err != nil {
		return nil, err
	}
	if time.Now().After(upload.ExpiresAt) {
		return nil, ErrResumableUploadExpired
	}
	if err := enforceUploadOwnership(upload, callerUserID); err != nil {
		return nil, err
	}
	return upload, nil
}

// AppendResumableUpload writes one chunk into a resumable upload session.
// callerUserID enforces ownership (nil = admin bypass).
// The bool return indicates whether the upload is now ready to finalize.
func (s *Service) AppendResumableUpload(ctx context.Context, id string, offset int64, callerUserID *string, src io.Reader) (*ResumableUpload, bool, error) {
	if id == "" {
		return nil, false, ErrResumableUploadNotFound
	}
	if s.pool == nil {
		return nil, false, fmt.Errorf("database pool is not configured")
	}
	if src == nil {
		return nil, false, fmt.Errorf("upload data is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin upload tx: %w", err)
	}
	defer tx.Rollback(ctx)

	tenantID := tenant.TenantFromContext(ctx)
	upload, err := getResumableUpload(ctx, tx, id, tenantID, true)
	if err != nil {
		return nil, false, err
	}

	if err := enforceUploadOwnership(upload, callerUserID); err != nil {
		return nil, false, err
	}
	if time.Now().After(upload.ExpiresAt) {
		return nil, false, ErrResumableUploadExpired
	}
	if upload.Status != resumableUploadStatusActive {
		return nil, false, ErrResumableUploadInvalidState
	}
	if offset != upload.UploadedSize {
		return nil, false, ErrResumableUploadOffsetMismatch
	}

	remaining := upload.TotalSize - upload.UploadedSize
	// Backend I/O stays inside the FOR UPDATE row-lock transaction so concurrent
	// appends (including cross-node) remain serialized by the Postgres row lock.
	written, err := s.stageAppendedChunk(ctx, upload, offset, remaining, src)
	if err != nil {
		return nil, false, err
	}
	upload.UploadedSize += written

	status := resumableUploadStatusActive
	shouldFinalize := false
	if upload.UploadedSize == upload.TotalSize {
		status = resumableUploadStatusFinalizing
		shouldFinalize = true
	}

	// Progress updates are tenant-scoped to match the locked session row.
	_, err = tx.Exec(ctx,
		`UPDATE _ayb_storage_uploads
		 SET uploaded_size = $1, status = $2, updated_at = NOW(), expires_at = NOW() + make_interval(secs => $3)
		 WHERE id = $4 AND tenant_id = $5`,
		upload.UploadedSize, status, int64(resumableUploadTTL.Seconds()), upload.ID, tenantID)
	if err != nil {
		return nil, false, fmt.Errorf("updating resumable upload progress: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("commit resumable upload tx: %w", err)
	}

	upload.Status = status
	return upload, shouldFinalize, nil
}

// moves a completed resumable upload's temporary file to backend storage and records it in the database. It applies row-level security constraints and cleans up the backend object if the database insert fails.
func (s *Service) finalizeUploadObject(ctx context.Context, upload *ResumableUpload) (*Object, error) {
	staged, err := s.backend.Get(ctx, upload.TenantID, resumableStagingBucket, upload.Path)
	if err != nil {
		return nil, fmt.Errorf("opening staged upload: %w", err)
	}
	defer staged.Close()

	size, err := s.backend.Put(ctx, upload.TenantID, upload.Bucket, upload.Name, staged)
	if err != nil {
		return nil, err
	}

	q, done, err := s.withRLS(ctx)
	if err != nil {
		_ = s.backend.Delete(ctx, upload.TenantID, upload.Bucket, upload.Name)
		return nil, err
	}

	var obj Object
	// Finalized resumable uploads use the same tenant-local object identity as
	// direct uploads, including the conflict target.
	err = q.QueryRow(ctx,
		`INSERT INTO _ayb_storage_objects (tenant_id, bucket, name, size, content_type, user_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (tenant_id, bucket, name) DO UPDATE
		 SET size = EXCLUDED.size, content_type = EXCLUDED.content_type, user_id = EXCLUDED.user_id, updated_at = NOW()
		 RETURNING id, bucket, name, size, content_type, user_id, created_at, updated_at`,
		upload.TenantID, upload.Bucket, upload.Name, size, upload.ContentType, upload.UserID,
	).Scan(&obj.ID, &obj.Bucket, &obj.Name, &obj.Size, &obj.ContentType, &obj.UserID, &obj.CreatedAt, &obj.UpdatedAt)
	if err != nil {
		_ = s.backend.Delete(ctx, upload.TenantID, upload.Bucket, upload.Name)
		_ = done(err)
		if isPermissionDenied(err) {
			return nil, ErrPermissionDenied
		}
		return nil, fmt.Errorf("recording resumable object: %w", err)
	}
	if err := done(nil); err != nil {
		_ = s.backend.Delete(ctx, upload.TenantID, upload.Bucket, upload.Name)
		return nil, fmt.Errorf("recording resumable object: %w", err)
	}

	return &obj, nil
}

// FinalizeResumableUpload moves a completed resumable upload into bucket storage.
// callerUserID enforces ownership (nil = admin bypass).
// Uses FOR UPDATE SKIP LOCKED to prevent concurrent finalize race conditions — the
// lock is held for the duration of the finalize rather than released before the work.
func (s *Service) FinalizeResumableUpload(ctx context.Context, id string, callerUserID *string) (*Object, error) {
	if id == "" {
		return nil, ErrResumableUploadNotFound
	}
	if s.pool == nil {
		return nil, fmt.Errorf("database pool is not configured")
	}
	tenantID := tenant.TenantFromContext(ctx)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin upload-finalize tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// SKIP LOCKED: if another goroutine is already finalizing this upload,
	// we get no rows rather than blocking, preventing double-finalize.
	row := tx.QueryRow(ctx, resumableUploadSelectQuery+" FOR UPDATE SKIP LOCKED", id, tenantID)
	upload, err := scanResumableUpload(row)
	if err != nil {
		return nil, err
	}

	if err := enforceUploadOwnership(upload, callerUserID); err != nil {
		return nil, err
	}
	if upload.Status != resumableUploadStatusFinalizing {
		return nil, ErrResumableUploadInvalidState
	}
	if upload.UploadedSize != upload.TotalSize {
		return nil, ErrResumableUploadInvalidState
	}
	if time.Now().After(upload.ExpiresAt) {
		return nil, ErrResumableUploadExpired
	}

	// Finalize: move temp file to backend and create object record.
	// The lock tx stays open to prevent concurrent finalize attempts.
	obj, err := s.finalizeUploadObject(ctx, upload)
	if err != nil {
		// Reset status so the client can retry.
		// Reset only this tenant's session; IDs alone are not an isolation boundary.
		_, _ = tx.Exec(ctx, `UPDATE _ayb_storage_uploads
			SET status = $1, updated_at = NOW()
			WHERE id = $2 AND tenant_id = $3`, resumableUploadStatusActive, id, tenantID)
		_ = tx.Commit(ctx)
		return nil, err
	}

	// Delete the upload record within the lock tx.
	// Remove only the tenant-scoped session that was locked above.
	if _, err := tx.Exec(ctx, `DELETE FROM _ayb_storage_uploads WHERE id = $1 AND tenant_id = $2`, id, tenantID); err != nil {
		return nil, fmt.Errorf("removing resumable upload: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit finalize tx: %w", err)
	}

	if err := s.backend.Delete(ctx, upload.TenantID, resumableStagingBucket, upload.Path); err != nil {
		s.logger.Warn("failed to remove resumable staging object", "path", upload.Path, "error", err)
	}

	return obj, nil
}

func (s *Service) CleanupExpiredResumableUploads(ctx context.Context) (int, error) {
	if s.pool == nil {
		return 0, fmt.Errorf("database pool is not configured")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin cleanup tx: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		DELETE FROM _ayb_storage_uploads
		WHERE expires_at < NOW()
		RETURNING path, tenant_id, user_id, total_size`)
	if err != nil {
		return 0, fmt.Errorf("deleting expired resumable uploads: %w", err)
	}
	defer rows.Close()

	type expiredUpload struct {
		path      string
		tenantID  string
		userID    *string
		totalSize int64
	}

	expiredUploads := make([]expiredUpload, 0)
	for rows.Next() {
		var upload expiredUpload
		if err := rows.Scan(&upload.path, &upload.tenantID, &upload.userID, &upload.totalSize); err != nil {
			return 0, fmt.Errorf("scanning expired resumable upload: %w", err)
		}
		expiredUploads = append(expiredUploads, upload)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("reading expired resumable uploads: %w", err)
	}
	rows.Close()

	for _, upload := range expiredUploads {
		if upload.userID != nil {
			if err := decrementUsageInTx(ctx, tx, upload.tenantID, *upload.userID, upload.totalSize); err != nil {
				return 0, fmt.Errorf("reclaiming resumable quota: %w", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit cleanup tx: %w", err)
	}

	for _, upload := range expiredUploads {
		if err := s.backend.Delete(ctx, upload.tenantID, resumableStagingBucket, upload.path); err != nil {
			s.logger.Warn("failed to remove resumable staging object", "path", upload.path, "error", err)
		}
	}

	return len(expiredUploads), nil
}
