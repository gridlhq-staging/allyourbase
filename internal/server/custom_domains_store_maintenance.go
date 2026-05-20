package server

import (
	"context"
	"fmt"
	"time"
)

// queryDomainBindings executes a SELECT query against _ayb_custom_domains and
// scans all resulting rows into DomainBinding slices. This is the single shared
// scan loop used by all bulk-list store methods.
func (s *DomainStore) queryDomainBindings(ctx context.Context, query string, args ...any) ([]DomainBinding, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []DomainBinding
	for rows.Next() {
		b, err := scanDomainBinding(rows)
		if err != nil {
			return nil, fmt.Errorf("scan domain row: %w", err)
		}
		items = append(items, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate domain rows: %w", err)
	}
	return items, nil
}

// ListDomainsForCertRenewal returns active domains with cert_expiry before renewBefore,
// ordered by cert_expiry ASC. Used by the cert renewal scheduled job.
func (s *DomainStore) ListDomainsForCertRenewal(ctx context.Context, renewBefore time.Time) ([]DomainBinding, error) {
	items, err := s.queryDomainBindings(ctx,
		`SELECT `+domainColumns+`
		FROM _ayb_custom_domains
		WHERE status = 'active' AND cert_expiry IS NOT NULL AND cert_expiry < $1
		ORDER BY cert_expiry ASC`,
		renewBefore,
	)
	if err != nil {
		return nil, fmt.Errorf("list domains for cert renewal: %w", err)
	}
	return items, nil
}

// TriggerVerification queues domain DNS verification if needed.
func (s *DomainStore) TriggerVerification(ctx context.Context, id string) (*DomainBinding, error) {
	domain, err := s.GetDomain(ctx, id)
	if err != nil {
		return nil, err
	}

	if domain.Status == StatusVerified || domain.Status == StatusActive {
		return domain, nil
	}
	if domain.Status != StatusPendingVerification && domain.Status != StatusVerificationFailed {
		return domain, nil
	}
	if s.jobSvc == nil {
		return domain, nil
	}
	if err := s.enqueueVerification(ctx, domain.ID); err != nil {
		return nil, err
	}
	return domain, nil
}

// ListDomainsForRouting returns all active, tombstoned, and verification_lapsed domain
// bindings ordered by hostname. This is an internal bulk-load query used for building
// the route table — no pagination.
func (s *DomainStore) ListDomainsForRouting(ctx context.Context) ([]DomainBinding, error) {
	items, err := s.queryDomainBindings(ctx,
		`SELECT `+domainColumns+` FROM _ayb_custom_domains
		WHERE status IN ('active', 'tombstoned', 'verification_lapsed')
		ORDER BY hostname ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list domains for routing: %w", err)
	}
	return items, nil
}

// ListDomainsForHealthCheck returns all active domains for health monitoring.
func (s *DomainStore) ListDomainsForHealthCheck(ctx context.Context) ([]DomainBinding, error) {
	items, err := s.queryDomainBindings(ctx,
		`SELECT `+domainColumns+` FROM _ayb_custom_domains
		WHERE status = 'active'
		ORDER BY hostname ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list domains for health check: %w", err)
	}
	return items, nil
}

// UpdateDomainHealth updates health_status and last_health_check for a domain.
func (s *DomainStore) UpdateDomainHealth(ctx context.Context, id string, healthStatus string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE _ayb_custom_domains SET health_status = $2, last_health_check = NOW(), updated_at = NOW() WHERE id = $1`,
		id, healthStatus,
	)
	if err != nil {
		return fmt.Errorf("update domain health: %w", err)
	}
	return nil
}

// ListDomainsForReverify returns all active domains for DNS re-verification.
// The reverify job runs daily; all active domains are checked each run.
func (s *DomainStore) ListDomainsForReverify(ctx context.Context) ([]DomainBinding, error) {
	items, err := s.queryDomainBindings(ctx,
		`SELECT `+domainColumns+` FROM _ayb_custom_domains
		WHERE status = 'active'
		ORDER BY hostname ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list domains for reverify: %w", err)
	}
	return items, nil
}

// IncrementReverifyFailures increments the reverify failure counter for a domain.
func (s *DomainStore) IncrementReverifyFailures(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE _ayb_custom_domains SET reverify_failures = reverify_failures + 1, updated_at = NOW()
		WHERE id = $1 AND status != 'tombstoned'`,
		id,
	)
	if err != nil {
		return fmt.Errorf("increment reverify failures: %w", err)
	}
	return nil
}

// ResetReverifyFailures resets the reverify failure counter after a successful DNS re-verification.
// Does NOT update last_health_check — that column is exclusively for cert health checks.
func (s *DomainStore) ResetReverifyFailures(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE _ayb_custom_domains SET reverify_failures = 0, updated_at = NOW()
		WHERE id = $1 AND status != 'tombstoned'`,
		id,
	)
	if err != nil {
		return fmt.Errorf("reset reverify failures: %w", err)
	}
	return nil
}

// ListLapsedDomainsForCleanup returns verification_lapsed domains past the 7-day grace period.
func (s *DomainStore) ListLapsedDomainsForCleanup(ctx context.Context) ([]DomainBinding, error) {
	items, err := s.queryDomainBindings(ctx,
		`SELECT `+domainColumns+` FROM _ayb_custom_domains
		WHERE status = 'verification_lapsed' AND updated_at < NOW() - interval '7 days'
		ORDER BY hostname ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list lapsed domains for cleanup: %w", err)
	}
	return items, nil
}

// ReapExpiredTombstones deletes domain bindings that have been tombstoned for more than 7 days.
func (s *DomainStore) ReapExpiredTombstones(ctx context.Context) (int64, error) {
	commandTag, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_custom_domains
		WHERE status = 'tombstoned' AND tombstoned_at < NOW() - interval '7 days'`,
	)
	if err != nil {
		return 0, fmt.Errorf("reap expired tombstones: %w", err)
	}
	return commandTag.RowsAffected(), nil
}
