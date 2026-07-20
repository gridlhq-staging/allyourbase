// Package status Defines probes for checking health of database and other services. Each probe reports health status, latency, and error details.
package status

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultStatusProbeTimeout = 2 * time.Second
	statusProbeTenantID       = ""
	statusProbeBucket         = "ayb_status_probe"
	statusProbeObject         = "reachability"
)

// Probe is one service health probe used by the checker.
type Probe interface {
	Name() ServiceName
	Check(ctx context.Context) ProbeResult
}

type pinger interface {
	Ping(ctx context.Context) error
}

type backendExistenceChecker interface {
	BackendExists(ctx context.Context, tenantID, bucket, name string) (bool, error)
}

// DatabaseProbe checks database reachability via pgx pool ping.
type DatabaseProbe struct {
	pinger  pinger
	timeout time.Duration
}

// NewDatabaseProbe constructs a database probe backed by a pgx pool.
func NewDatabaseProbe(pool *pgxpool.Pool) *DatabaseProbe {
	if pool == nil {
		return &DatabaseProbe{timeout: defaultStatusProbeTimeout}
	}
	return &DatabaseProbe{pinger: pool, timeout: defaultStatusProbeTimeout}
}

func (p *DatabaseProbe) Name() ServiceName {
	return Database
}

// Check pings the database via the configured pgx pool and returns a ProbeResult with health status, latency, and any error. If the probe or its pinger is nil, an unhealthy result is returned without attempting to ping.
func (p *DatabaseProbe) Check(ctx context.Context) ProbeResult {
	start := time.Now().UTC()
	if p == nil || p.pinger == nil {
		return ProbeResult{
			Service:   Database,
			Healthy:   false,
			Error:     "database pool not configured",
			CheckedAt: start,
		}
	}

	checkCtx, cancel := context.WithTimeout(ctx, probeTimeout(p.timeout))
	defer cancel()
	if err := checkCtx.Err(); err != nil {
		return ProbeResult{
			Service:   Database,
			Healthy:   false,
			Error:     err.Error(),
			CheckedAt: start,
		}
	}

	err := p.pinger.Ping(checkCtx)
	latency := time.Since(start)
	if err != nil {
		return ProbeResult{
			Service:   Database,
			Healthy:   false,
			Latency:   latency,
			Error:     err.Error(),
			CheckedAt: start,
		}
	}

	return ProbeResult{
		Service:   Database,
		Healthy:   true,
		Latency:   latency,
		CheckedAt: start,
	}
}

// StorageProbe checks whether the storage backend can answer an object
// existence request. The sentinel object does not need to exist; a clean
// false,nil result still proves backend reachability.
type StorageProbe struct {
	checker backendExistenceChecker
	timeout time.Duration
}

func NewStorageProbe(checker backendExistenceChecker) *StorageProbe {
	return &StorageProbe{checker: checker, timeout: defaultStatusProbeTimeout}
}

func (p *StorageProbe) Name() ServiceName {
	return Storage
}

func (p *StorageProbe) Check(ctx context.Context) ProbeResult {
	start := time.Now().UTC()
	if p == nil || p.checker == nil {
		return ProbeResult{
			Service:   Storage,
			Healthy:   false,
			Error:     "storage backend not configured",
			CheckedAt: start,
		}
	}

	checkCtx, cancel := context.WithTimeout(ctx, probeTimeout(p.timeout))
	defer cancel()
	if err := checkCtx.Err(); err != nil {
		return ProbeResult{
			Service:   Storage,
			Healthy:   false,
			Error:     err.Error(),
			CheckedAt: start,
		}
	}

	_, err := p.checker.BackendExists(checkCtx, statusProbeTenantID, statusProbeBucket, statusProbeObject)
	latency := time.Since(start)
	if err != nil {
		return ProbeResult{
			Service:   Storage,
			Healthy:   false,
			Latency:   latency,
			Error:     "storage backend check failed",
			CheckedAt: start,
		}
	}

	return ProbeResult{
		Service:   Storage,
		Healthy:   true,
		Latency:   latency,
		CheckedAt: start,
	}
}

func probeTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return defaultStatusProbeTimeout
	}
	return timeout
}
