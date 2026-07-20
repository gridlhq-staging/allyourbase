package status

import (
	"testing"
	"time"
)

func TestDeriveStatus(t *testing.T) {
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

	t.Run("all healthy operational", func(t *testing.T) {
		results := []ProbeResult{
			{Service: Database, Healthy: true, Latency: 100 * time.Millisecond, CheckedAt: now},
			{Service: Storage, Healthy: true, Latency: 120 * time.Millisecond, CheckedAt: now},
			{Service: Auth, Healthy: true, Latency: 140 * time.Millisecond, CheckedAt: now},
		}
		if got := DeriveStatus(results); got != Operational {
			t.Fatalf("DeriveStatus() = %q, want %q", got, Operational)
		}
	})

	t.Run("unhealthy database is major outage regardless of denominator", func(t *testing.T) {
		results := []ProbeResult{
			{Service: Database, Healthy: false, CheckedAt: now},
			{Service: Storage, Healthy: true, CheckedAt: now},
			{Service: Auth, Healthy: true, CheckedAt: now},
			{Service: Realtime, Healthy: true, CheckedAt: now},
			{Service: Functions, Healthy: true, CheckedAt: now},
		}
		if got := DeriveStatus(results); got != MajorOutage {
			t.Fatalf("DeriveStatus() = %q, want %q", got, MajorOutage)
		}
	})

	t.Run("healthy database plus unhealthy storage is partial outage", func(t *testing.T) {
		results := []ProbeResult{
			{Service: Database, Healthy: true, CheckedAt: now},
			{Service: Storage, Healthy: false, CheckedAt: now},
			{Service: Auth, Healthy: true, CheckedAt: now},
		}
		if got := DeriveStatus(results); got != PartialOutage {
			t.Fatalf("DeriveStatus() = %q, want %q", got, PartialOutage)
		}
	})

	t.Run("all healthy but slow degraded", func(t *testing.T) {
		results := []ProbeResult{
			{Service: Database, Healthy: true, Latency: SlowProbeThreshold + 10*time.Millisecond, CheckedAt: now},
			{Service: Storage, Healthy: true, Latency: 100 * time.Millisecond, CheckedAt: now},
		}
		if got := DeriveStatus(results); got != Degraded {
			t.Fatalf("DeriveStatus() = %q, want %q", got, Degraded)
		}
	})

	t.Run("empty results are major outage", func(t *testing.T) {
		if got := DeriveStatus(nil); got != MajorOutage {
			t.Fatalf("DeriveStatus() = %q, want %q", got, MajorOutage)
		}
	})

	t.Run("single unhealthy is major outage", func(t *testing.T) {
		// When there's exactly one probe and it's unhealthy, unhealthy (1) > healthy (0)
		// so the result should be major outage.
		results := []ProbeResult{
			{Service: Database, Healthy: false},
		}
		if got := DeriveStatus(results); got != MajorOutage {
			t.Fatalf("DeriveStatus(single unhealthy) = %q, want %q", got, MajorOutage)
		}
	})

	t.Run("equal split without database failure is partial outage", func(t *testing.T) {
		results := []ProbeResult{
			{Service: Database, Healthy: true},
			{Service: Storage, Healthy: true},
			{Service: Auth, Healthy: false},
			{Service: Realtime, Healthy: false},
		}
		if got := DeriveStatus(results); got != PartialOutage {
			t.Fatalf("DeriveStatus(2 healthy, 2 unhealthy) = %q, want %q", got, PartialOutage)
		}
	})

	t.Run("slow unhealthy ignores latency", func(t *testing.T) {
		// A slow but unhealthy probe should not trigger "degraded" — latency only
		// matters for healthy probes.
		results := []ProbeResult{
			{Service: Database, Healthy: true, Latency: 50 * time.Millisecond},
			{Service: Storage, Healthy: false, Latency: 5 * time.Second},
		}
		if got := DeriveStatus(results); got != PartialOutage {
			t.Fatalf("DeriveStatus(one healthy, one slow+unhealthy) = %q, want %q", got, PartialOutage)
		}
	})
}
