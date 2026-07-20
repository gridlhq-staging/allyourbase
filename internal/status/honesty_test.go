package status

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDeriveStatusEmptyResultsIsMajorOutage(t *testing.T) {
	if got := DeriveStatus(nil); got != MajorOutage {
		t.Fatalf("DeriveStatus(nil) = %q, want %q (no evidence must not report healthy)", got, MajorOutage)
	}
}

func TestDeriveStatusSingleRealFailureIsNotMasked(t *testing.T) {
	results := []ProbeResult{
		{Service: Database, Healthy: false, Error: "database pool not configured"},
		{Service: Storage, Healthy: true},
		{Service: Auth, Healthy: true},
		{Service: Realtime, Healthy: true},
		{Service: Functions, Healthy: true},
	}
	if got := DeriveStatus(results); got != MajorOutage {
		t.Fatalf("DeriveStatus(1 db failure, 4 healthy) = %q, want %q (real DB outage must not be masked)", got, MajorOutage)
	}
}

func TestStorageProbeIndeterminateIsUnhealthy(t *testing.T) {
	backend := &recordingBackendExistenceChecker{err: errors.New("storage backend unavailable")}
	probe := NewStorageProbe(backend)

	res := probe.Check(context.Background())
	if res.Service != Storage || res.Healthy || res.Error == "" {
		t.Fatalf("Storage probe with backend error = {Service:%q Healthy:%v Error:%q}, want {Service:%q Healthy:false Error:non-empty}",
			res.Service, res.Healthy, res.Error, Storage)
	}
	backend.assertSentinelArgs(t)
}

func TestStatusRollupWithIndeterminateProductionProbesIsMajorOutage(t *testing.T) {
	probes := []Probe{
		&DatabaseProbe{pinger: &probePinger{}},
		NewStorageProbe(&recordingBackendExistenceChecker{err: errors.New("storage backend unavailable")}),
	}
	checker := NewChecker(probes, NewStatusHistory(1), time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	snapshot := checker.RunOnce(ctx)
	if snapshot.Status != MajorOutage {
		t.Fatalf("RunOnce(canceled ctx with database and storage probes) status = %q, want %q",
			snapshot.Status, MajorOutage)
	}
}

func TestStatusRollupWithHealthyDatabaseAndStorageIsOperational(t *testing.T) {
	probes := []Probe{
		&DatabaseProbe{pinger: &probePinger{}},
		NewStorageProbe(&recordingBackendExistenceChecker{exists: false}),
	}
	checker := NewChecker(probes, NewStatusHistory(1), time.Minute)

	snapshot := checker.RunOnce(context.Background())
	if snapshot.Status != Operational {
		t.Fatalf("RunOnce(database healthy, storage reachable) status = %q, want %q",
			snapshot.Status, Operational)
	}
	if len(snapshot.Services) != 2 {
		t.Fatalf("services len = %d, want 2", len(snapshot.Services))
	}
	if snapshot.Services[0].Service != Database || !snapshot.Services[0].Healthy {
		t.Fatalf("database result = {Service:%q Healthy:%v Error:%q}, want healthy database",
			snapshot.Services[0].Service, snapshot.Services[0].Healthy, snapshot.Services[0].Error)
	}
	if snapshot.Services[1].Service != Storage || !snapshot.Services[1].Healthy {
		t.Fatalf("storage result = {Service:%q Healthy:%v Error:%q}, want healthy storage",
			snapshot.Services[1].Service, snapshot.Services[1].Healthy, snapshot.Services[1].Error)
	}
}
