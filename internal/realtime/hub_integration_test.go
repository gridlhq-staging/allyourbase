//go:build integration

package realtime_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/pgnotify"
	"github.com/allyourbase/ayb/internal/realtime"
	"github.com/allyourbase/ayb/internal/testutil"
)

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func TestIntegrationHubBusDeliversForeignTableEventAndSuppressesSelfEcho(t *testing.T) {
	logger := testutil.DiscardLogger()
	busA := pgnotify.NewBus(sharedPG.Pool, sharedPG.ConnString, logger)
	busB := pgnotify.NewBus(sharedPG.Pool, sharedPG.ConnString, logger)
	hubA := realtime.NewHubWithBus(logger, busA)
	t.Cleanup(hubA.Close)
	hubB := realtime.NewHubWithBus(logger, busB)
	t.Cleanup(hubB.Close)

	clientA := hubA.Subscribe(map[string]bool{"users": true})

	foreignEvent := realtime.Event{
		Action: "create",
		Table:  "users",
		Record: map[string]any{"id": float64(101), "name": "foreign"},
	}
	publishUntilReceived(t, hubB, clientA, foreignEvent)

	clientB := hubB.Subscribe(map[string]bool{"users": true})
	selfEvent := &realtime.Event{
		Action: "update",
		Table:  "users",
		Record: map[string]any{"id": 202, "name": "self"},
	}
	hubB.Publish(selfEvent)
	assertNextEvent(t, clientB, selfEvent)
	assertNoExtraEvent(t, clientB, 200*time.Millisecond)
}

func TestIntegrationHubBusOversizedTableEventStaysLocalAndPreservesLaterFanout(t *testing.T) {
	logger := testutil.DiscardLogger()
	busA := pgnotify.NewBus(sharedPG.Pool, sharedPG.ConnString, logger)
	busB := pgnotify.NewBus(sharedPG.Pool, sharedPG.ConnString, logger)
	hubA := realtime.NewHubWithBus(logger, busA)
	t.Cleanup(hubA.Close)
	hubB := realtime.NewHubWithBus(logger, busB)
	t.Cleanup(hubB.Close)

	localClient := hubA.Subscribe(map[string]bool{"users": true})
	foreignClient := hubB.Subscribe(map[string]bool{"users": true})

	oversizedEvent := &realtime.Event{
		Action: "create",
		Table:  "users",
		Record: map[string]any{
			"id":   303,
			"name": "oversized-local",
			"blob": strings.Repeat("x", 9000),
		},
	}
	hubA.Publish(oversizedEvent)
	assertNextEvent(t, localClient, oversizedEvent)
	assertNoExtraEvent(t, foreignClient, 3*time.Second)

	smallEvent := realtime.Event{
		Action: "update",
		Table:  "users",
		Record: map[string]any{"id": float64(404), "name": "small-after-oversized"},
	}
	publishUntilReceived(t, hubA, foreignClient, smallEvent)
	assertNoExtraEvent(t, foreignClient, 3*time.Second)
}

// TestIntegrationHubBusPreservesTenantIsolationAcrossNodes proves that when two
// hubs share a pgnotify.Bus, a tenant-scoped event published on one node is
// delivered to the same-tenant subscriber on the other node and suppressed for a
// different-tenant subscriber on that same table, and that an empty-tenant
// wildcard event still fans out across nodes to every subscriber.
func TestIntegrationHubBusPreservesTenantIsolationAcrossNodes(t *testing.T) {
	logger := testutil.DiscardLogger()
	busA := pgnotify.NewBus(sharedPG.Pool, sharedPG.ConnString, logger)
	busB := pgnotify.NewBus(sharedPG.Pool, sharedPG.ConnString, logger)
	hubA := realtime.NewHubWithBus(logger, busA)
	t.Cleanup(hubA.Close)
	hubB := realtime.NewHubWithBus(logger, busB)
	t.Cleanup(hubB.Close)

	// Same table ("posts"), different tenants, subscribed on the remote node B.
	tenantAClient := hubB.SubscribeWithFilter(map[string]bool{"posts": true}, nil, "tenant-a")
	tenantBClient := hubB.SubscribeWithFilter(map[string]bool{"posts": true}, nil, "tenant-b")

	// A tenant-a event published on node A must reach the tenant-a client on
	// node B and never the tenant-b client.
	tenantAEvent := realtime.Event{
		Action:   "create",
		Table:    "posts",
		TenantID: "tenant-a",
		Record:   map[string]any{"id": float64(1), "name": "scoped"},
	}
	publishUntilReceived(t, hubA, tenantAClient, tenantAEvent)
	assertNoExtraEvent(t, tenantBClient, 500*time.Millisecond)

	// An empty-tenant wildcard event still fans out across nodes to both
	// subscribers regardless of their tenant.
	wildcardClientA := hubB.SubscribeWithFilter(map[string]bool{"widgets": true}, nil, "tenant-a")
	wildcardClientB := hubB.SubscribeWithFilter(map[string]bool{"widgets": true}, nil, "tenant-b")
	wildcardEvent := realtime.Event{
		Action:   "create",
		Table:    "widgets",
		TenantID: "",
		Record:   map[string]any{"id": float64(2), "name": "wildcard"},
	}
	// Both clients must observe the same cross-node wildcard event. Retry until
	// the bus listener is warm, then confirm the second client also received it.
	publishUntilReceived(t, hubA, wildcardClientA, wildcardEvent)
	assertNextEvent(t, wildcardClientB, &wildcardEvent)
}

func publishUntilReceived(t *testing.T, hub *realtime.Hub, client *realtime.Client, event realtime.Event) {
	t.Helper()

	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for attempt := 1; ; attempt++ {
		next := event
		next.Record = cloneRecord(event.Record)
		next.Record["attempt"] = float64(attempt)
		hub.Publish(&next)

		select {
		case got := <-client.Events():
			assertEvent(t, got, &next)
			return
		case <-ticker.C:
		case <-deadline:
			t.Fatalf("timed out waiting for foreign hub event through pgnotify")
		}
	}
}

func assertNextEvent(t *testing.T, client *realtime.Client, want *realtime.Event) {
	t.Helper()

	select {
	case got := <-client.Events():
		assertEvent(t, got, want)
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for event %#v", want)
	}
}

func assertNoExtraEvent(t *testing.T, client *realtime.Client, duration time.Duration) {
	t.Helper()

	select {
	case got := <-client.Events():
		t.Fatalf("unexpected duplicate event: %#v", got)
	case <-time.After(duration):
	}
}

func assertEvent(t *testing.T, got *realtime.Event, want *realtime.Event) {
	t.Helper()

	testutil.Equal(t, want.Action, got.Action)
	testutil.Equal(t, want.Table, got.Table)
	for key, wantValue := range want.Record {
		testutil.Equal(t, fmt.Sprint(wantValue), fmt.Sprint(got.Record[key]))
	}
}

func cloneRecord(record map[string]any) map[string]any {
	cloned := make(map[string]any, len(record))
	for key, value := range record {
		cloned[key] = value
	}
	return cloned
}
