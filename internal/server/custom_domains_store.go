package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/allyourbase/ayb/internal/audit"
	"github.com/allyourbase/ayb/internal/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgUniqueViolation is the PostgreSQL error code for unique constraint violations.
const pgUniqueViolation = "23505"

// domainColumns is the SQL column list used by all domain queries and RETURNING clauses.
const domainColumns = `id, hostname, environment, status, verification_token, cert_ref, cert_expiry, redirect_mode, last_error, tombstoned_at, created_at, updated_at, health_status, last_health_check, reverify_failures`

// domainScanner is satisfied by both pgx.Row and pgx.Rows.
type domainScanner interface {
	Scan(dest ...any) error
}

// scanDomainBinding scans a row into a DomainBinding.
func scanDomainBinding(s domainScanner) (DomainBinding, error) {
	var b DomainBinding
	err := s.Scan(
		&b.ID, &b.Hostname, &b.Environment, &b.Status, &b.VerificationToken,
		&b.CertRef, &b.CertExpiry, &b.RedirectMode, &b.LastError, &b.TombstonedAt,
		&b.CreatedAt, &b.UpdatedAt, &b.HealthStatus, &b.LastHealthCheck, &b.ReverifyFailures,
	)
	return b, err
}

// DomainStore implements domainManager using a PostgreSQL connection pool.
type DomainStore struct {
	pool      *pgxpool.Pool
	auditSink audit.Sink
	jobSvc    *jobs.Service
}

// NewDomainStore constructs a DomainStore backed by the given pool and audit sink.
func NewDomainStore(pool *pgxpool.Pool, auditSink audit.Sink) *DomainStore {
	return &DomainStore{pool: pool, auditSink: auditSink}
}

// SetJobService wires a job service for enqueueing domain verification jobs.
func (s *DomainStore) SetJobService(svc *jobs.Service) {
	s.jobSvc = svc
}

// enqueueVerification marshals a fresh verification payload and enqueues it.
func (s *DomainStore) enqueueVerification(ctx context.Context, domainID string) error {
	payload, err := json.Marshal(domainVerifyPayload{
		DomainID:  domainID,
		StartedAt: time.Now(),
		Attempt:   1,
	})
	if err != nil {
		return fmt.Errorf("marshal domain DNS verification payload: %w", err)
	}
	_, err = s.jobSvc.Enqueue(ctx, JobTypeDomainDNSVerify, payload, jobs.EnqueueOpts{MaxAttempts: 1})
	return err
}

// enqueueCertProvision marshals a cert provision payload and enqueues it.
func (s *DomainStore) enqueueCertProvision(ctx context.Context, domainID string) error {
	payload, err := json.Marshal(domainCertProvisionPayload{DomainID: domainID})
	if err != nil {
		return fmt.Errorf("marshal domain cert provision payload: %w", err)
	}
	_, err = s.jobSvc.Enqueue(ctx, JobTypeDomainCertProvision, payload, jobs.EnqueueOpts{MaxAttempts: 3})
	return err
}

// CreateDomain inserts a new domain binding with pending_verification status.
// It generates a verification token and populates VerificationRecord on the returned binding.
func (s *DomainStore) CreateDomain(ctx context.Context, hostname, environment, redirectMode string) (*DomainBinding, error) {
	if environment == "" {
		environment = "production"
	}

	token := generateVerificationToken()

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var redirectModeArg any
	if redirectMode != "" {
		redirectModeArg = redirectMode
	}

	b, err := scanDomainBinding(tx.QueryRow(ctx,
		`INSERT INTO _ayb_custom_domains (hostname, environment, verification_token, redirect_mode)
		VALUES ($1, $2, $3, $4)
		RETURNING `+domainColumns,
		hostname, environment, token, redirectModeArg,
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return nil, ErrDomainHostnameConflict
		}
		return nil, fmt.Errorf("insert domain: %w", err)
	}

	if err := s.auditSink.LogMutationWithQuerier(ctx, tx, audit.AuditEntry{
		TableName: "_ayb_custom_domains",
		RecordID:  b.ID,
		Operation: "INSERT",
		NewValues: map[string]any{
			"hostname":    b.Hostname,
			"environment": b.Environment,
			"status":      b.Status,
		},
	}); err != nil {
		return nil, fmt.Errorf("audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	b.populateVerificationRecord()

	if s.jobSvc != nil {
		if err := s.enqueueVerification(ctx, b.ID); err != nil {
			slog.Default().Warn("failed to enqueue domain DNS verification job", "error", err, "domain_id", b.ID)
		}
	}

	return &b, nil
}

// GetDomain retrieves a domain binding by ID.
func (s *DomainStore) GetDomain(ctx context.Context, id string) (*DomainBinding, error) {
	b, err := scanDomainBinding(s.pool.QueryRow(ctx,
		`SELECT `+domainColumns+` FROM _ayb_custom_domains WHERE id = $1`, id,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDomainNotFound
		}
		return nil, fmt.Errorf("get domain: %w", err)
	}

	b.populateVerificationRecord()
	return &b, nil
}

// ListDomains returns a paginated list of domain bindings ordered by created_at DESC.
func (s *DomainStore) ListDomains(ctx context.Context, page, perPage int) (*DomainBindingListResult, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_custom_domains`).Scan(&total); err != nil {
		return nil, fmt.Errorf("count domains: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := s.pool.Query(ctx,
		`SELECT `+domainColumns+` FROM _ayb_custom_domains ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		perPage, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list domains: %w", err)
	}
	defer rows.Close()

	items := []DomainBinding{}
	for rows.Next() {
		b, err := scanDomainBinding(rows)
		if err != nil {
			return nil, fmt.Errorf("scan domain row: %w", err)
		}
		b.populateVerificationRecord()
		items = append(items, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate domain rows: %w", err)
	}

	totalPages := total / perPage
	if total%perPage != 0 {
		totalPages++
	}

	return &DomainBindingListResult{
		Items:      items,
		Page:       page,
		PerPage:    perPage,
		TotalItems: total,
		TotalPages: totalPages,
	}, nil
}

// DeleteDomain soft-deletes a domain binding by setting its status to tombstoned.
// After commit, enqueues a cert revoke job to stop certmagic from renewing the cert.
func (s *DomainStore) DeleteDomain(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var hostname string
	err = tx.QueryRow(ctx, `
		UPDATE _ayb_custom_domains
		SET status = 'tombstoned', tombstoned_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status != 'tombstoned'
		RETURNING hostname`, id,
	).Scan(&hostname)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDomainNotFound
		}
		return fmt.Errorf("tombstone domain: %w", err)
	}

	if err := s.auditSink.LogMutationWithQuerier(ctx, tx, audit.AuditEntry{
		TableName: "_ayb_custom_domains",
		RecordID:  id,
		Operation: "UPDATE",
		NewValues: map[string]any{"status": "tombstoned"},
	}); err != nil {
		return fmt.Errorf("audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	if s.jobSvc != nil {
		payload, err := json.Marshal(domainCertRevokePayload{DomainID: id, Hostname: hostname})
		if err == nil {
			if _, enqErr := s.jobSvc.Enqueue(ctx, JobTypeDomainCertRevoke, payload, jobs.EnqueueOpts{MaxAttempts: 3}); enqErr != nil {
				slog.Default().Warn("failed to enqueue domain cert revoke job", "error", enqErr, "domain_id", id)
			}
		}
	}

	return nil
}

// UpdateDomainStatus transitions status and last_error for a binding.
func (s *DomainStore) UpdateDomainStatus(ctx context.Context, id string, status DomainStatus, lastError *string) (*DomainBinding, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	b, err := scanDomainBinding(tx.QueryRow(ctx,
		`UPDATE _ayb_custom_domains
		SET status = $2, last_error = $3, updated_at = NOW()
		WHERE id = $1 AND status != 'tombstoned'
		RETURNING `+domainColumns,
		id, status, lastError,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDomainNotFound
		}
		return nil, fmt.Errorf("update domain status: %w", err)
	}

	if err := s.auditSink.LogMutationWithQuerier(ctx, tx, audit.AuditEntry{
		TableName: "_ayb_custom_domains",
		RecordID:  id,
		Operation: "UPDATE",
		NewValues: map[string]any{
			"status":     status,
			"last_error": lastError,
		},
	}); err != nil {
		return nil, fmt.Errorf("audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	b.populateVerificationRecord()

	if b.Status == StatusVerified && lastError == nil && s.jobSvc != nil {
		if err := s.enqueueCertProvision(ctx, b.ID); err != nil {
			slog.Default().Warn("failed to enqueue domain cert provision job", "error", err, "domain_id", b.ID)
		}
	}

	return &b, nil
}

// SetDomainCert transitions a verified or active domain to active status and stores
// certificate metadata. Transitions verified → active on first cert issuance;
// refreshes cert_ref and cert_expiry on renewal.
func (s *DomainStore) SetDomainCert(ctx context.Context, id string, certRef string, certExpiry time.Time) (*DomainBinding, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	b, err := scanDomainBinding(tx.QueryRow(ctx,
		`UPDATE _ayb_custom_domains
		SET status = 'active', cert_ref = $2, cert_expiry = $3, last_error = NULL, updated_at = NOW()
		WHERE id = $1 AND status IN ('verified', 'active')
		RETURNING `+domainColumns,
		id, certRef, certExpiry,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDomainNotFound
		}
		return nil, fmt.Errorf("set domain cert: %w", err)
	}

	if err := s.auditSink.LogMutationWithQuerier(ctx, tx, audit.AuditEntry{
		TableName: "_ayb_custom_domains",
		RecordID:  id,
		Operation: "UPDATE",
		NewValues: map[string]any{
			"status":      "active",
			"cert_ref":    certRef,
			"cert_expiry": certExpiry,
		},
	}); err != nil {
		return nil, fmt.Errorf("audit log: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	b.populateVerificationRecord()
	return &b, nil
}
