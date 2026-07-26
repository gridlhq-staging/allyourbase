package pgnotify

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/allyourbase/ayb/internal/backoff"
)

const (
	maxLogicalNameBytes           = 32
	maxPostgresNotifyPayloadBytes = 7999
	physicalChannelPrefix         = "ayb_pgnotify_"
	listenTimeout                 = 30 * time.Second
)

var (
	ErrInvalidChannel  = errors.New("invalid pgnotify channel")
	ErrPayloadTooLarge = errors.New("pgnotify payload too large")
)

type Bus struct {
	pool       *pgxpool.Pool
	connString string
	logger     *slog.Logger
	nodeID     string
	listener   *rawListener
}

type envelope struct {
	NodeID string          `json:"n"`
	Kind   string          `json:"k"`
	Data   json.RawMessage `json:"d"`
}

// NewBus creates a PostgreSQL-backed notification bus.
func NewBus(pool *pgxpool.Pool, connString string, logger *slog.Logger) *Bus {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	listenerBackoff := backoff.Config{
		Base: 250 * time.Millisecond,
		Cap:  5 * time.Second,
	}
	b := &Bus{
		pool:       pool,
		connString: connString,
		logger:     logger,
		nodeID:     newNodeID(),
	}
	b.listener = newRawListener(connString, logger, listenerBackoff)
	return b
}

func (b *Bus) NodeID() string {
	return b.nodeID
}

// Publish sends a notification to subscribers of a logical channel.
func (b *Bus) Publish(ctx context.Context, name string, kind string, data any) error {
	channel, err := physicalChannelName(name)
	if err != nil {
		return err
	}
	payload, err := encodeEnvelope(b.nodeID, kind, data)
	if err != nil {
		return err
	}
	if b.pool == nil {
		return errors.New("pgnotify publish: nil pool")
	}
	if _, err := b.pool.Exec(ctx, "select pg_notify($1, $2)", channel, payload); err != nil {
		return fmt.Errorf("pgnotify publish: %w", err)
	}
	return nil
}

// Subscribe listens for notifications from other bus instances on a logical channel.
func (b *Bus) Subscribe(ctx context.Context, name string, handler func(kind string, data json.RawMessage)) error {
	channel, err := physicalChannelName(name)
	if err != nil {
		return err
	}
	if handler == nil {
		return errors.New("pgnotify subscribe: nil handler")
	}
	return b.ListenRaw(ctx, channel, func(payload string) {
		b.handleNotification(payload, handler)
	})
}

func (b *Bus) listenerBackendPID(name string) uint32 {
	channel, err := physicalChannelName(name)
	if err != nil {
		return 0
	}
	return b.listener.backendPID(channel)
}

// WaitForListener waits until this bus has an active LISTEN backend for name.
func (b *Bus) WaitForListener(ctx context.Context, name string) error {
	if err := validateLogicalName(name); err != nil {
		return err
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if b.listenerBackendPID(name) != 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// ListenRaw listens on an unprefixed PostgreSQL channel and passes raw payloads
// to handler. It shares the bus listener reconnect and backend PID tracking
// without applying the bus envelope or self-echo filtering.
func (b *Bus) ListenRaw(ctx context.Context, channel string, handler func(payload string), opts ...RawListenOption) error {
	return b.listener.listen(ctx, channel, handler, opts...)
}

func (b *Bus) handleNotification(payload string, handler func(kind string, data json.RawMessage)) {
	msg, err := decodeEnvelope(payload)
	if err != nil {
		b.logger.Warn("pgnotify invalid payload", "error", err)
		return
	}
	if msg.NodeID == b.nodeID {
		return
	}
	handler(msg.Kind, append(json.RawMessage(nil), msg.Data...))
}

func validateLogicalName(name string) error {
	if len(name) == 0 || len(name) > maxLogicalNameBytes {
		return fmt.Errorf("%w: %q", ErrInvalidChannel, name)
	}
	for _, ch := range name {
		if ch >= 'a' && ch <= 'z' {
			continue
		}
		if ch >= '0' && ch <= '9' {
			continue
		}
		if ch == '_' {
			continue
		}
		return fmt.Errorf("%w: %q", ErrInvalidChannel, name)
	}
	return nil
}

func physicalChannelName(name string) (string, error) {
	if err := validateLogicalName(name); err != nil {
		return "", err
	}
	return physicalChannelPrefix + name, nil
}

func encodeEnvelope(nodeID string, kind string, data any) (string, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("pgnotify encode data: %w", err)
	}
	payload, err := json.Marshal(envelope{
		NodeID: nodeID,
		Kind:   kind,
		Data:   raw,
	})
	if err != nil {
		return "", fmt.Errorf("pgnotify encode envelope: %w", err)
	}
	if len(payload) > maxPostgresNotifyPayloadBytes {
		return "", fmt.Errorf("%w: %d bytes", ErrPayloadTooLarge, len(payload))
	}
	return string(payload), nil
}

func decodeEnvelope(payload string) (envelope, error) {
	var msg envelope
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		return envelope{}, fmt.Errorf("pgnotify decode envelope: %w", err)
	}
	if msg.NodeID == "" {
		return envelope{}, errors.New("pgnotify decode envelope: missing node id")
	}
	if msg.Data == nil {
		return envelope{}, errors.New("pgnotify decode envelope: missing data")
	}
	return msg, nil
}

func newNodeID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("pgnotify node id: %v", err))
	}
	return hex.EncodeToString(b[:])
}
