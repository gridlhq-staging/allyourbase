//go:build integration

package pgnotify

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

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

func TestIntegrationBusDeliversCrossNodeAndSuppressesSelfEcho(t *testing.T) {
	busA := NewBus(sharedPG.Pool, sharedPG.ConnString, testutil.DiscardLogger())
	busB := NewBus(sharedPG.Pool, sharedPG.ConnString, testutil.DiscardLogger())
	deliveries := make(chan busDelivery, 4)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- busA.Subscribe(ctx, "events", func(kind string, data json.RawMessage) {
			deliveries <- busDelivery{kind: kind, data: append(json.RawMessage(nil), data...)}
		})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Subscribe returned %v, want context.Canceled", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Subscribe did not return after context cancellation")
		}
	})

	waitForNoDelivery(t, deliveries, 100*time.Millisecond)
	if err := busA.Publish(context.Background(), "events", "self", json.RawMessage(`{"from":"a"}`)); err != nil {
		t.Fatalf("self publish: %v", err)
	}
	waitForNoDelivery(t, deliveries, 150*time.Millisecond)

	waitForPublishDelivery(t, busB, "events", deliveries, busDelivery{
		kind: "remote",
		data: json.RawMessage(`{"from":"b","n":7}`),
	})
}

func TestIntegrationBusReconnectsAfterListenerBackendTermination(t *testing.T) {
	busA := NewBus(sharedPG.Pool, sharedPG.ConnString, testutil.DiscardLogger())
	busB := NewBus(sharedPG.Pool, sharedPG.ConnString, testutil.DiscardLogger())
	deliveries := make(chan busDelivery, 1)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- busA.Subscribe(ctx, "reconnect", func(kind string, data json.RawMessage) {
			deliveries <- busDelivery{kind: kind, data: append(json.RawMessage(nil), data...)}
		})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Subscribe returned %v, want context.Canceled", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Subscribe did not return after context cancellation")
		}
	})

	initialPID := waitForListenerBackendPID(t, busA, "reconnect")
	if _, err := sharedPG.Pool.Exec(context.Background(), "select pg_terminate_backend($1)", initialPID); err != nil {
		t.Fatalf("terminate listener backend %d: %v", initialPID, err)
	}
	waitForListenerBackendPIDChange(t, busA, "reconnect", initialPID)

	want := busDelivery{
		kind: "after_reconnect",
		data: json.RawMessage(`{"ok":true,"attempt":1}`),
	}
	if err := busB.Publish(context.Background(), "reconnect", want.kind, want.data); err != nil {
		t.Fatalf("publish after reconnect: %v", err)
	}
	waitForDelivery(t, deliveries, want)
}

func TestIntegrationRejectedPublishAttemptsDoNotDeliver(t *testing.T) {
	busA := NewBus(sharedPG.Pool, sharedPG.ConnString, testutil.DiscardLogger())
	busB := NewBus(sharedPG.Pool, sharedPG.ConnString, testutil.DiscardLogger())
	deliveries := make(chan busDelivery, 2)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- busA.Subscribe(ctx, "rejects", func(kind string, data json.RawMessage) {
			deliveries <- busDelivery{kind: kind, data: append(json.RawMessage(nil), data...)}
		})
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	waitForNoDelivery(t, deliveries, 100*time.Millisecond)
	if err := busB.Publish(context.Background(), "bad-name", "invalid", json.RawMessage(`{}`)); !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("invalid publish error = %v, want ErrInvalidChannel", err)
	}
	waitForNoDelivery(t, deliveries, 150*time.Millisecond)

	err := busB.Publish(context.Background(), "rejects", "large", strings.Repeat("x", maxPostgresNotifyPayloadBytes))
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("oversized publish error = %v, want ErrPayloadTooLarge", err)
	}
	waitForNoDelivery(t, deliveries, 150*time.Millisecond)
}

func TestIntegrationSubscribeBlocksUntilContextCanceled(t *testing.T) {
	bus := NewBus(sharedPG.Pool, sharedPG.ConnString, testutil.DiscardLogger())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- bus.Subscribe(ctx, "blocking", func(string, json.RawMessage) {})
	}()

	select {
	case err := <-done:
		t.Fatalf("Subscribe returned before cancellation: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Subscribe returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Subscribe did not unblock after context cancellation")
	}
}

type busDelivery struct {
	kind string
	data json.RawMessage
}

func waitForPublishDelivery(t *testing.T, bus *Bus, channel string, deliveries <-chan busDelivery, want busDelivery) {
	t.Helper()

	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		if err := bus.Publish(context.Background(), channel, want.kind, want.data); err != nil {
			t.Fatalf("publish: %v", err)
		}

		select {
		case got := <-deliveries:
			assertBusDelivery(t, got, want)
			return
		case <-ticker.C:
		case <-deadline:
			t.Fatalf("timed out waiting for delivery (%q, %s)", want.kind, want.data)
		}
	}
}

func waitForDelivery(t *testing.T, deliveries <-chan busDelivery, want busDelivery) {
	t.Helper()

	select {
	case got := <-deliveries:
		assertBusDelivery(t, got, want)
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for delivery (%q, %s)", want.kind, want.data)
	}
}

func waitForListenerBackendPID(t *testing.T, bus *Bus, channel string) uint32 {
	t.Helper()

	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		if pid := bus.listenerBackendPID(channel); pid != 0 {
			return pid
		}

		select {
		case <-ticker.C:
		case <-deadline:
			t.Fatalf("timed out waiting for listener backend PID on %q", channel)
		}
	}
}

func waitForListenerBackendPIDChange(t *testing.T, bus *Bus, channel string, previous uint32) uint32 {
	t.Helper()

	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		if pid := bus.listenerBackendPID(channel); pid != 0 && pid != previous {
			return pid
		}

		select {
		case <-ticker.C:
		case <-deadline:
			t.Fatalf("timed out waiting for listener backend PID on %q to change from %d", channel, previous)
		}
	}
}

func assertBusDelivery(t *testing.T, got busDelivery, want busDelivery) {
	t.Helper()

	if got.kind != want.kind || string(got.data) != string(want.data) {
		t.Fatalf("delivery = (%q, %s), want (%q, %s)", got.kind, got.data, want.kind, want.data)
	}
}

func waitForNoDelivery(t *testing.T, deliveries <-chan busDelivery, duration time.Duration) {
	t.Helper()

	select {
	case got := <-deliveries:
		t.Fatalf("unexpected delivery: (%q, %s)", got.kind, got.data)
	case <-time.After(duration):
	}
}
