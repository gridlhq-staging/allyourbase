package pgnotify

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestBusNodeIDStablePerInstance(t *testing.T) {
	bus := NewBus(nil, "", testutil.DiscardLogger())

	first := bus.NodeID()
	if first == "" {
		t.Fatal("NodeID() is empty")
	}
	if got := bus.NodeID(); got != first {
		t.Fatalf("NodeID() changed: first %q, second %q", first, got)
	}

	other := NewBus(nil, "", testutil.DiscardLogger())
	if other.NodeID() == first {
		t.Fatalf("two bus instances used the same node id %q", first)
	}
}

func TestLogicalChannelValidation(t *testing.T) {
	valid := []string{"a", "abc_123", strings.Repeat("a", 32)}
	for _, name := range valid {
		if err := validateLogicalName(name); err != nil {
			t.Fatalf("validateLogicalName(%q): %v", name, err)
		}
	}

	invalid := []string{"", "A", "has-dash", "has.dot", "has space", strings.Repeat("a", 33)}
	for _, name := range invalid {
		if err := validateLogicalName(name); !errors.Is(err, ErrInvalidChannel) {
			t.Fatalf("validateLogicalName(%q) error = %v, want ErrInvalidChannel", name, err)
		}
	}
}

func TestPhysicalChannelName(t *testing.T) {
	got, err := physicalChannelName("tenant_events")
	if err != nil {
		t.Fatalf("physicalChannelName: %v", err)
	}
	if want := "ayb_pgnotify_tenant_events"; got != want {
		t.Fatalf("physicalChannelName() = %q, want %q", got, want)
	}
}

func TestRawChannelValidation(t *testing.T) {
	valid := []string{"a", "abc_123", strings.Repeat("a", 63)}
	for _, channel := range valid {
		if err := validateRawChannelName(channel); err != nil {
			t.Fatalf("validateRawChannelName(%q): %v", channel, err)
		}
	}

	invalid := []string{"", "_leading", "A", "1starts_digit", "has-dash", "has.dot", "has space", strings.Repeat("a", 64)}
	for _, channel := range invalid {
		if err := validateRawChannelName(channel); !errors.Is(err, ErrInvalidChannel) {
			t.Fatalf("validateRawChannelName(%q) error = %v, want ErrInvalidChannel", channel, err)
		}
	}
}

func TestRawListenReconnectPolicyDefaultsToRetryForever(t *testing.T) {
	cfg := defaultRawListenConfig()

	if cfg.reconnectFailurePolicy != RawListenReconnectRetryForever {
		t.Fatalf("reconnectFailurePolicy = %v, want RawListenReconnectRetryForever", cfg.reconnectFailurePolicy)
	}
}

func TestRawListenReconnectPolicyCanReturnFailure(t *testing.T) {
	cfg := defaultRawListenConfig()
	WithRawListenReconnectFailurePolicy(RawListenReconnectReturnFailure)(&cfg)

	if cfg.reconnectFailurePolicy != RawListenReconnectReturnFailure {
		t.Fatalf("reconnectFailurePolicy = %v, want RawListenReconnectReturnFailure", cfg.reconnectFailurePolicy)
	}
}

func TestRawListenStartHandlerCanBeConfigured(t *testing.T) {
	cfg := defaultRawListenConfig()
	handler := func(context.Context) {}
	WithRawListenStartHandler(handler)(&cfg)

	if cfg.onStart == nil {
		t.Fatal("onStart is nil, want configured handler")
	}
}

func TestListenRawRejectsInvalidChannelBeforeDatabase(t *testing.T) {
	bus := NewBus(nil, "", testutil.DiscardLogger())

	err := bus.ListenRaw(context.Background(), "invalid-name", func(string) {})
	if !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("ListenRaw error = %v, want ErrInvalidChannel", err)
	}
}

func TestListenRawRejectsNilHandlerBeforeDatabase(t *testing.T) {
	bus := NewBus(nil, "", testutil.DiscardLogger())

	err := bus.ListenRaw(context.Background(), "events", nil)
	if err == nil || !strings.Contains(err.Error(), "nil handler") {
		t.Fatalf("ListenRaw error = %v, want nil handler error", err)
	}
}

func TestEnvelopeRoundTripPreservesNodeKindAndData(t *testing.T) {
	data := json.RawMessage(`{"z":2,"a":[true,"x"]}`)

	payload, err := encodeEnvelope("node-1", "changed", data)
	if err != nil {
		t.Fatalf("encodeEnvelope: %v", err)
	}
	if strings.Contains(payload, " ") || strings.Contains(payload, "\n") {
		t.Fatalf("payload is not compact JSON: %q", payload)
	}
	if want := `{"n":"node-1","k":"changed","d":{"z":2,"a":[true,"x"]}}`; payload != want {
		t.Fatalf("payload = %q, want %q", payload, want)
	}

	envelope, err := decodeEnvelope(payload)
	if err != nil {
		t.Fatalf("decodeEnvelope: %v", err)
	}
	if envelope.NodeID != "node-1" {
		t.Fatalf("NodeID = %q, want node-1", envelope.NodeID)
	}
	if envelope.Kind != "changed" {
		t.Fatalf("Kind = %q, want changed", envelope.Kind)
	}
	if string(envelope.Data) != `{"z":2,"a":[true,"x"]}` {
		t.Fatalf("Data = %s, want caller JSON data preserved", envelope.Data)
	}
}

func TestEnvelopeRejectsOversizedPayload(t *testing.T) {
	_, err := encodeEnvelope("node-1", "large", strings.Repeat("x", maxPostgresNotifyPayloadBytes))
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("encodeEnvelope error = %v, want ErrPayloadTooLarge", err)
	}
}

func TestPublishRejectsInvalidChannelBeforeDatabase(t *testing.T) {
	bus := NewBus(nil, "", testutil.DiscardLogger())

	err := bus.Publish(context.Background(), "invalid-name", "kind", json.RawMessage(`{}`))
	if !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("Publish error = %v, want ErrInvalidChannel", err)
	}
}

func TestWaitForListenerRejectsInvalidChannelBeforeWaiting(t *testing.T) {
	bus := NewBus(nil, "", testutil.DiscardLogger())

	err := bus.WaitForListener(context.Background(), "invalid-name")
	if !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("WaitForListener error = %v, want ErrInvalidChannel", err)
	}
}

func TestWaitForListenerReturnsContextErrorWhenNoListenerStarts(t *testing.T) {
	bus := NewBus(nil, "", testutil.DiscardLogger())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := bus.WaitForListener(ctx, "events")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForListener error = %v, want context deadline exceeded", err)
	}
}
