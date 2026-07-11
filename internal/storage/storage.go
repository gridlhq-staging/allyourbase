package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/allyourbase/ayb/internal/observability"
	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sentinel errors.
var (
	ErrNotFound         = errors.New("object not found")
	ErrAlreadyExists    = errors.New("object already exists")
	ErrInvalidBucket    = errors.New("invalid bucket name")
	ErrInvalidName      = errors.New("invalid object name")
	ErrPermissionDenied = errors.New("permission denied")
	ErrBucketNotFound   = errors.New("bucket not found")
	ErrBucketNotEmpty   = errors.New("bucket has objects")
)

// Backend is the interface for file storage backends.
type Backend interface {
	Put(ctx context.Context, tenantID, bucket, name string, r io.Reader) (int64, error)
	Get(ctx context.Context, tenantID, bucket, name string) (io.ReadCloser, error)
	Delete(ctx context.Context, tenantID, bucket, name string) error
	Exists(ctx context.Context, tenantID, bucket, name string) (bool, error)
}

// Object represents a stored file's metadata.
type Object struct {
	ID          string    `json:"id"`
	Bucket      string    `json:"bucket"`
	Name        string    `json:"name"`
	Size        int64     `json:"size"`
	ContentType string    `json:"contentType"`
	UserID      *string   `json:"userId,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Service handles file storage operations.
type Service struct {
	pool              *pgxpool.Pool
	backend           Backend
	signKey           []byte
	logger            *slog.Logger
	defaultQuotaBytes int64
	eventHandlers     []StorageEventHandler
}

// SignedURLValidation is the storage-owned result of signed URL parsing and
// HMAC verification.
type SignedURLValidation struct {
	Valid    bool
	TenantID string
}

// NewService creates a new storage service.
func NewService(pool *pgxpool.Pool, backend Backend, signKey string, logger *slog.Logger, defaultQuotaBytes int64) *Service {
	return &Service{
		pool:              pool,
		backend:           backend,
		signKey:           []byte(signKey),
		logger:            logger,
		defaultQuotaBytes: defaultQuotaBytes,
	}
}

// RegisterEventHandler adds a handler that will be notified of storage events.
func (s *Service) RegisterEventHandler(h StorageEventHandler) {
	s.eventHandlers = append(s.eventHandlers, h)
}

// dispatchEvent notifies all registered handlers of a storage event.
// Handler errors are logged but not propagated to the caller.
func (s *Service) dispatchEvent(ctx context.Context, event StorageEvent) {
	for _, h := range s.eventHandlers {
		if err := h.OnStorageEvent(ctx, event); err != nil {
			s.logger.Error("storage event handler failed",
				"bucket", event.Bucket,
				"name", event.Name,
				"operation", event.Operation,
				"error", err,
			)
		}
	}
}

// Upload stores a file and records its metadata.
func (s *Service) Upload(ctx context.Context, bucket, name, contentType string, userID *string, r io.Reader) (*Object, error) {
	ctx, span := otel.Tracer("ayb/storage").Start(ctx, "storage.upload",
		trace.WithAttributes(
			attribute.String("storage.bucket", bucket),
			attribute.String("storage.object", name),
		),
	)
	defer span.End()
	if err := validateBucket(bucket); err != nil {
		observability.RecordSpanError(span, err)
		return nil, err
	}
	if err := validateName(name); err != nil {
		observability.RecordSpanError(span, err)
		return nil, err
	}
	tenantID := tenant.TenantFromContext(ctx)

	size, err := s.backend.Put(ctx, tenantID, bucket, name, r)
	if err != nil {
		observability.RecordSpanError(span, err)
		return nil, fmt.Errorf("storing file: %w", err)
	}

	q, done, err := s.withRLS(ctx)
	if err != nil {
		_ = s.backend.Delete(ctx, tenantID, bucket, name)
		observability.RecordSpanError(span, err)
		return nil, fmt.Errorf("setting storage rls context: %w", err)
	}

	var obj Object
	// tenant_id is part of the object identity so same bucket/name uploads in
	// different tenants cannot overwrite each other's metadata.
	err = q.QueryRow(ctx,
		`INSERT INTO _ayb_storage_objects (tenant_id, bucket, name, size, content_type, user_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (tenant_id, bucket, name) DO UPDATE
		 SET size = EXCLUDED.size, content_type = EXCLUDED.content_type, updated_at = NOW()
		 RETURNING id, bucket, name, size, content_type, user_id, created_at, updated_at`,
		tenantID, bucket, name, size, contentType, userID,
	).Scan(&obj.ID, &obj.Bucket, &obj.Name, &obj.Size, &obj.ContentType,
		&obj.UserID, &obj.CreatedAt, &obj.UpdatedAt)
	if err != nil {
		_ = done(err)
		// Clean up the stored file on DB error.
		_ = s.backend.Delete(ctx, tenantID, bucket, name)
		if isPermissionDenied(err) {
			observability.RecordSpanError(span, ErrPermissionDenied)
			return nil, ErrPermissionDenied
		}
		observability.RecordSpanError(span, err)
		return nil, fmt.Errorf("recording metadata: %w", err)
	}
	if err := done(nil); err != nil {
		_ = s.backend.Delete(ctx, tenantID, bucket, name)
		observability.RecordSpanError(span, err)
		return nil, fmt.Errorf("recording metadata: %w", err)
	}

	s.logger.Info("file uploaded", "bucket", bucket, "name", name, "size", size)

	s.dispatchEvent(ctx, StorageEvent{
		Bucket:      bucket,
		Name:        name,
		Operation:   OperationUpload,
		Size:        size,
		ContentType: contentType,
	})

	return &obj, nil
}

// Download retrieves a file's content and metadata.
func (s *Service) Download(ctx context.Context, bucket, name string) (io.ReadCloser, *Object, error) {
	ctx, span := otel.Tracer("ayb/storage").Start(ctx, "storage.download",
		trace.WithAttributes(
			attribute.String("storage.bucket", bucket),
			attribute.String("storage.object", name),
		),
	)
	defer span.End()
	obj, err := s.GetObject(ctx, bucket, name)
	if err != nil {
		observability.RecordSpanError(span, err)
		return nil, nil, err
	}

	reader, err := s.backend.Get(ctx, tenant.TenantFromContext(ctx), bucket, name)
	if err != nil {
		observability.RecordSpanError(span, err)
		return nil, nil, fmt.Errorf("reading file: %w", err)
	}

	return reader, obj, nil
}

// GetObject returns the metadata for a stored file.
func (s *Service) GetObject(ctx context.Context, bucket, name string) (*Object, error) {
	q, done, err := s.withRLS(ctx)
	if err != nil {
		return nil, fmt.Errorf("setting storage rls context: %w", err)
	}

	var obj Object
	// Scope by tenant_id first; bucket/name alone is intentionally reusable
	// across tenants.
	err = q.QueryRow(ctx,
		`SELECT id, bucket, name, size, content_type, user_id, created_at, updated_at
		 FROM _ayb_storage_objects
		 WHERE tenant_id = $1 AND bucket = $2 AND name = $3`,
		tenant.TenantFromContext(ctx), bucket, name,
	).Scan(&obj.ID, &obj.Bucket, &obj.Name, &obj.Size, &obj.ContentType,
		&obj.UserID, &obj.CreatedAt, &obj.UpdatedAt)
	if err != nil {
		_ = done(err)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if isPermissionDenied(err) {
			return nil, ErrPermissionDenied
		}
		return nil, fmt.Errorf("querying object: %w", err)
	}
	if err := done(nil); err != nil {
		return nil, fmt.Errorf("querying object: %w", err)
	}
	return &obj, nil
}

// DeleteObject removes a file and its metadata.
func (s *Service) DeleteObject(ctx context.Context, bucket, name string) error {
	ctx, span := otel.Tracer("ayb/storage").Start(ctx, "storage.delete",
		trace.WithAttributes(
			attribute.String("storage.bucket", bucket),
			attribute.String("storage.object", name),
		),
	)
	defer span.End()
	if err := validateBucket(bucket); err != nil {
		observability.RecordSpanError(span, err)
		return err
	}
	if err := validateName(name); err != nil {
		observability.RecordSpanError(span, err)
		return err
	}
	tenantID := tenant.TenantFromContext(ctx)

	q, done, err := s.withRLS(ctx)
	if err != nil {
		observability.RecordSpanError(span, err)
		return fmt.Errorf("setting storage rls context: %w", err)
	}

	// Delete only this tenant's metadata row; physical cleanup uses the same
	// tenantID below so bytes are deleted from the matching namespace.
	tag, err := q.Exec(ctx,
		`DELETE FROM _ayb_storage_objects
		 WHERE tenant_id = $1 AND bucket = $2 AND name = $3`,
		tenantID, bucket, name,
	)
	if err != nil {
		_ = done(err)
		if isPermissionDenied(err) {
			observability.RecordSpanError(span, ErrPermissionDenied)
			return ErrPermissionDenied
		}
		observability.RecordSpanError(span, err)
		return fmt.Errorf("deleting metadata: %w", err)
	}
	if tag.RowsAffected() == 0 {
		_ = done(nil)
		observability.RecordSpanError(span, ErrNotFound)
		return ErrNotFound
	}
	if err := done(nil); err != nil {
		observability.RecordSpanError(span, err)
		return fmt.Errorf("deleting metadata: %w", err)
	}

	if err := s.backend.Delete(ctx, tenantID, bucket, name); err != nil {
		s.logger.Error("failed to delete file from backend", "bucket", bucket, "name", name, "error", err)
	}

	s.logger.Info("file deleted", "bucket", bucket, "name", name)

	s.dispatchEvent(ctx, StorageEvent{
		Bucket:    bucket,
		Name:      name,
		Operation: OperationDelete,
	})

	return nil
}

// ListObjects lists files in a bucket with pagination.
func (s *Service) ListObjects(ctx context.Context, bucket string, prefix string, limit, offset int) ([]Object, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	q, done, err := s.withRLS(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("setting storage rls context: %w", err)
	}
	tenantID := tenant.TenantFromContext(ctx)

	// Count total.
	var total int
	// Listing is tenant-scoped because object names are reusable across tenants.
	countQuery := `SELECT COUNT(*) FROM _ayb_storage_objects WHERE tenant_id = $1 AND bucket = $2`
	countArgs := []any{tenantID, bucket}
	if prefix != "" {
		countQuery += ` AND name LIKE $3 ESCAPE '\'`
		countArgs = append(countArgs, escapeLikePrefix(prefix)+"%")
	}
	if err := q.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		_ = done(err)
		if isPermissionDenied(err) {
			return nil, 0, ErrPermissionDenied
		}
		return nil, 0, fmt.Errorf("counting objects: %w", err)
	}

	// Fetch page.
	listQuery := `SELECT id, bucket, name, size, content_type, user_id, created_at, updated_at
		FROM _ayb_storage_objects WHERE tenant_id = $1 AND bucket = $2`
	listArgs := []any{tenantID, bucket}
	if prefix != "" {
		listQuery += ` AND name LIKE $3 ESCAPE '\'`
		listArgs = append(listArgs, escapeLikePrefix(prefix)+"%")
	}
	listQuery += ` ORDER BY name`
	listQuery += fmt.Sprintf(` LIMIT %d OFFSET %d`, limit, offset)

	rows, err := q.Query(ctx, listQuery, listArgs...)
	if err != nil {
		_ = done(err)
		if isPermissionDenied(err) {
			return nil, 0, ErrPermissionDenied
		}
		return nil, 0, fmt.Errorf("listing objects: %w", err)
	}
	defer rows.Close()

	var objects []Object
	for rows.Next() {
		var obj Object
		if err := rows.Scan(&obj.ID, &obj.Bucket, &obj.Name, &obj.Size, &obj.ContentType,
			&obj.UserID, &obj.CreatedAt, &obj.UpdatedAt); err != nil {
			_ = done(err)
			return nil, 0, fmt.Errorf("scanning object: %w", err)
		}
		objects = append(objects, obj)
	}
	if err := rows.Err(); err != nil {
		_ = done(err)
		return nil, 0, fmt.Errorf("iterating objects: %w", err)
	}
	if err := done(nil); err != nil {
		return nil, 0, fmt.Errorf("listing objects: %w", err)
	}

	return objects, total, nil
}

func isPermissionDenied(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42501"
}

// SignURL generates a signed URL token for time-limited access.
func (s *Service) SignURL(ctx context.Context, bucket, name string, expiry time.Duration) string {
	exp := time.Now().Add(expiry).Unix()
	tenantID := strings.TrimSpace(tenant.TenantFromContext(ctx))
	sig := s.signSignedURLPayload(bucket, name, exp, tenantID)
	if tenantID == "" {
		return fmt.Sprintf("exp=%d&sig=%s", exp, sig)
	}
	values := url.Values{}
	values.Set("exp", strconv.FormatInt(exp, 10))
	values.Set("sig", sig)
	values.Set("tenant", tenantID)
	return values.Encode()
}

// ValidateSignedURL checks that a signed URL token is valid and not expired.
func (s *Service) ValidateSignedURL(bucket, name string, values url.Values) SignedURLValidation {
	expStr := values.Get("exp")
	sig := values.Get("sig")
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return SignedURLValidation{}
	}
	if time.Now().Unix() > exp {
		return SignedURLValidation{}
	}
	tenantID := strings.TrimSpace(values.Get("tenant"))
	expected := s.signSignedURLPayload(bucket, name, exp, tenantID)
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return SignedURLValidation{}
	}
	return SignedURLValidation{Valid: true, TenantID: tenantID}
}

func (s *Service) signSignedURLPayload(bucket, name string, exp int64, tenantID string) string {
	payload := fmt.Sprintf("%s/%s:%d", bucket, name, exp)
	if tenantID != "" {
		payload = fmt.Sprintf("%s/%s:%d:%s", bucket, name, exp, tenantID)
	}
	mac := hmac.New(sha256.New, s.signKey)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func validateBucket(bucket string) error {
	if bucket == "" {
		return fmt.Errorf("%w: bucket name is required", ErrInvalidBucket)
	}
	if len(bucket) > 63 {
		return fmt.Errorf("%w: bucket name too long (max 63)", ErrInvalidBucket)
	}
	for _, c := range bucket {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return fmt.Errorf("%w: bucket name must contain only lowercase letters, digits, hyphens, underscores", ErrInvalidBucket)
		}
	}
	return nil
}

// escapeLikePrefix escapes SQL LIKE metacharacters (%, _, \) in a user-provided
// prefix so they are treated as literal characters. The result should be used with
// the ESCAPE '\' clause.
func escapeLikePrefix(prefix string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(prefix)
}

func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: object name is required", ErrInvalidName)
	}
	if len(name) > 1024 {
		return fmt.Errorf("%w: object name too long (max 1024)", ErrInvalidName)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("%w: object name must not contain \"..\"", ErrInvalidName)
	}
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("%w: object name must not start with \"/\"", ErrInvalidName)
	}
	return nil
}
