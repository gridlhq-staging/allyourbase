package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/allyourbase/ayb/internal/tenant"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Bucket describes storage bucket metadata and ACL settings.
type Bucket struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Public    bool      `json:"public"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// CreateBucket creates a bucket with the configured access control setting.
func (s *Service) CreateBucket(ctx context.Context, name string, public bool) (*Bucket, error) {
	if err := validateBucket(name); err != nil {
		return nil, err
	}
	tenantID := tenant.TenantFromContext(ctx)

	// The staging bucket is reserved for in-progress resumable upload bytes; a
	// caller-created bucket of the same name would collide with staged blobs.
	if name == resumableStagingBucket {
		return nil, fmt.Errorf("%w: bucket name is reserved", ErrInvalidBucket)
	}

	var b Bucket
	// Bucket names are tenant-local; global uniqueness would block pooled tenants
	// from using the same bucket name.
	err := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_storage_buckets (tenant_id, name, public)
		 VALUES ($1, $2, $3)
		 RETURNING id, name, public, created_at, updated_at`,
		tenantID, name, public,
	).Scan(&b.ID, &b.Name, &b.Public, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrAlreadyExists
		}
		return nil, fmt.Errorf("inserting bucket: %w", err)
	}

	return &b, nil
}

// GetBucket returns bucket metadata by name.
func (s *Service) GetBucket(ctx context.Context, name string) (*Bucket, error) {
	if err := validateBucket(name); err != nil {
		return nil, err
	}
	tenantID := tenant.TenantFromContext(ctx)

	var b Bucket
	// Bucket metadata and ACLs are tenant-owned even when bucket names match.
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, public, created_at, updated_at
		 FROM _ayb_storage_buckets
		 WHERE tenant_id = $1 AND name = $2`,
		tenantID, name,
	).Scan(&b.ID, &b.Name, &b.Public, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, wrapBucketLookupError(err)
	}

	return &b, nil
}

// ListBuckets returns all configured buckets, sorted by name.
func (s *Service) ListBuckets(ctx context.Context) ([]Bucket, error) {
	tenantID := tenant.TenantFromContext(ctx)
	// Bucket listings must not reveal another tenant's bucket names or ACLs.
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, public, created_at, updated_at
		 FROM _ayb_storage_buckets
		 WHERE tenant_id = $1
		 ORDER BY name`,
		tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing buckets: %w", err)
	}
	defer rows.Close()

	var buckets []Bucket
	for rows.Next() {
		var b Bucket
		if err := rows.Scan(&b.ID, &b.Name, &b.Public, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning bucket row: %w", err)
		}
		buckets = append(buckets, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating buckets: %w", err)
	}

	return buckets, nil
}

// UpdateBucket updates bucket metadata.
func (s *Service) UpdateBucket(ctx context.Context, name string, public bool) (*Bucket, error) {
	if err := validateBucket(name); err != nil {
		return nil, err
	}
	tenantID := tenant.TenantFromContext(ctx)

	var b Bucket
	// Update the tenant-local bucket row only; name alone is not unique anymore.
	err := s.pool.QueryRow(ctx,
		`UPDATE _ayb_storage_buckets
		 SET public = $3, updated_at = NOW()
		 WHERE tenant_id = $1 AND name = $2
		 RETURNING id, name, public, created_at, updated_at`,
		tenantID, name, public,
	).Scan(&b.ID, &b.Name, &b.Public, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, wrapBucketLookupError(err)
	}

	return &b, nil
}

// DeleteBucket deletes a bucket; when force=true it also removes all objects in the bucket.
func (s *Service) DeleteBucket(ctx context.Context, name string, force bool) error {
	if err := validateBucket(name); err != nil {
		return err
	}
	tenantID := tenant.TenantFromContext(ctx)

	if !force {
		var objectCount int
		// Non-force delete checks only this tenant's objects so another tenant's
		// same-named bucket cannot block deletion.
		if err := s.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM _ayb_storage_objects WHERE tenant_id = $1 AND bucket = $2`,
			tenantID, name,
		).Scan(&objectCount); err != nil {
			return fmt.Errorf("checking bucket object count: %w", err)
		}
		if objectCount > 0 {
			return ErrBucketNotEmpty
		}
		if err := s.deleteBucketRow(ctx, name); err != nil {
			return err
		}
		return nil
	}

	// Force delete: use a transaction so object + bucket row deletes are atomic.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin force-delete tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Force delete selects only this tenant's objects; backend cleanup below
	// uses the same tenantID namespace.
	objRows, err := tx.Query(ctx,
		`SELECT name FROM _ayb_storage_objects WHERE tenant_id = $1 AND bucket = $2`,
		tenantID, name,
	)
	if err != nil {
		return fmt.Errorf("querying bucket objects: %w", err)
	}
	defer objRows.Close()

	var objectNames []string
	for objRows.Next() {
		var objectName string
		if err := objRows.Scan(&objectName); err != nil {
			return fmt.Errorf("scanning object name: %w", err)
		}
		objectNames = append(objectNames, objectName)
	}
	if err := objRows.Err(); err != nil {
		return fmt.Errorf("iterating bucket objects: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM _ayb_storage_objects WHERE tenant_id = $1 AND bucket = $2`,
		tenantID, name,
	); err != nil {
		return fmt.Errorf("deleting bucket objects: %w", err)
	}

	tag, err := tx.Exec(ctx,
		`DELETE FROM _ayb_storage_buckets WHERE tenant_id = $1 AND name = $2`,
		tenantID, name,
	)
	if err != nil {
		return fmt.Errorf("deleting bucket metadata: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrBucketNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit force-delete tx: %w", err)
	}

	// Backend file cleanup happens after the DB transaction commits.
	// Log and continue on individual file errors to avoid leaving partial state.
	for _, objectName := range objectNames {
		if err := s.backend.Delete(ctx, tenantID, name, objectName); err != nil {
			s.logger.Error("failed to delete backend file during force bucket delete",
				"bucket", name, "object", objectName, "error", err)
		}
	}

	return nil
}

func (s *Service) deleteBucketRow(ctx context.Context, name string) error {
	// Bucket row deletion is tenant-scoped because bucket names are reusable.
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_storage_buckets WHERE tenant_id = $1 AND name = $2`,
		tenant.TenantFromContext(ctx), name,
	)
	if err != nil {
		return fmt.Errorf("deleting bucket metadata: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrBucketNotFound
	}
	return nil
}

func wrapBucketLookupError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrBucketNotFound
	}
	return fmt.Errorf("querying bucket: %w", err)
}
