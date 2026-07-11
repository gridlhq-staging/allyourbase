package pgnotify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/allyourbase/ayb/internal/backoff"
)

const maxPostgresChannelNameBytes = 63

// RawListenReconnectFailurePolicy controls what happens when a reconnect attempt fails.
type RawListenReconnectFailurePolicy int

const (
	RawListenReconnectRetryForever RawListenReconnectFailurePolicy = iota
	RawListenReconnectReturnFailure
)

// RawListenOption customizes a raw PostgreSQL LISTEN loop.
type RawListenOption func(*rawListenConfig)

type rawListenConfig struct {
	listenTimeout          time.Duration
	onStart                func(context.Context)
	onTimeout              func(context.Context)
	reconnectFailurePolicy RawListenReconnectFailurePolicy
}

type rawListener struct {
	connString string
	logger     *slog.Logger
	backoff    backoff.Config

	mu          sync.RWMutex
	backendPIDs map[string]uint32
}

func newRawListener(connString string, logger *slog.Logger, backoffConfig backoff.Config) *rawListener {
	return &rawListener{
		connString:  connString,
		logger:      logger,
		backoff:     backoffConfig,
		backendPIDs: make(map[string]uint32),
	}
}

func defaultRawListenConfig() rawListenConfig {
	return rawListenConfig{
		listenTimeout:          listenTimeout,
		reconnectFailurePolicy: RawListenReconnectRetryForever,
	}
}

// WithRawListenTimeout overrides the maximum wait per PostgreSQL notification read.
func WithRawListenTimeout(timeout time.Duration) RawListenOption {
	return func(cfg *rawListenConfig) {
		if timeout > 0 {
			cfg.listenTimeout = timeout
		}
	}
}

// WithRawListenStartHandler runs handler after each successful LISTEN setup.
func WithRawListenStartHandler(handler func(context.Context)) RawListenOption {
	return func(cfg *rawListenConfig) {
		cfg.onStart = handler
	}
}

// WithRawListenTimeoutHandler runs handler after each notification wait timeout.
func WithRawListenTimeoutHandler(handler func(context.Context)) RawListenOption {
	return func(cfg *rawListenConfig) {
		cfg.onTimeout = handler
	}
}

// WithRawListenReconnectFailurePolicy overrides how failed reconnects are handled.
func WithRawListenReconnectFailurePolicy(policy RawListenReconnectFailurePolicy) RawListenOption {
	return func(cfg *rawListenConfig) {
		switch policy {
		case RawListenReconnectRetryForever, RawListenReconnectReturnFailure:
			cfg.reconnectFailurePolicy = policy
		}
	}
}

func (l *rawListener) listen(
	ctx context.Context,
	channel string,
	handler func(payload string),
	opts ...RawListenOption,
) error {
	if err := validateRawChannelName(channel); err != nil {
		return err
	}
	if handler == nil {
		return errors.New("pgnotify raw listen: nil handler")
	}

	cfg := defaultRawListenConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	conn, err := l.connectAndListen(ctx, channel)
	if err != nil {
		return err
	}
	if cfg.onStart != nil {
		cfg.onStart(ctx)
	}
	defer func() {
		l.closeConnection(channel, conn)
	}()

	attempt := 1
	for {
		err := l.waitForNotifications(ctx, conn, cfg, handler)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		l.logger.Warn("pgnotify listener connection lost", "channel", channel, "error", err)
		l.closeConnection(channel, conn)
		conn = nil

		delay := backoff.Exponential(attempt, l.backoff)
		attempt++
		if err := sleepContext(ctx, delay); err != nil {
			return err
		}

		conn, err = l.connectAndListen(ctx, channel)
		if err != nil {
			l.logger.Warn("pgnotify listener reconnect failed", "channel", channel, "error", err)
			if cfg.reconnectFailurePolicy == RawListenReconnectReturnFailure {
				return err
			}
			continue
		}
		if cfg.onStart != nil {
			cfg.onStart(ctx)
		}
		attempt = 1
	}
}

func (l *rawListener) connectAndListen(ctx context.Context, channel string) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, l.connString)
	if err != nil {
		return nil, fmt.Errorf("pgnotify connect: %w", err)
	}
	if _, err := conn.Exec(ctx, "LISTEN "+channel); err != nil {
		conn.Close(context.Background())
		return nil, fmt.Errorf("pgnotify listen: %w", err)
	}
	l.setBackendPID(channel, conn.PgConn().PID())
	l.logger.Debug("pgnotify listening", "channel", channel)
	return conn, nil
}

func (l *rawListener) waitForNotifications(
	ctx context.Context,
	conn *pgx.Conn,
	cfg rawListenConfig,
	handler func(payload string),
) error {
	if conn == nil {
		return errors.New("pgnotify wait: nil conn")
	}
	for {
		waitCtx, cancel := context.WithTimeout(ctx, cfg.listenTimeout)
		notification, err := conn.WaitForNotification(waitCtx)
		cancel()

		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if waitCtx.Err() == context.DeadlineExceeded {
				if cfg.onTimeout != nil {
					cfg.onTimeout(ctx)
				}
				continue
			}
			return fmt.Errorf("pgnotify wait: %w", err)
		}
		handler(notification.Payload)
	}
}

func (l *rawListener) backendPID(channel string) uint32 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.backendPIDs[channel]
}

func (l *rawListener) setBackendPID(channel string, pid uint32) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.backendPIDs[channel] = pid
}

func (l *rawListener) clearBackendPID(channel string, pid uint32) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.backendPIDs[channel] == pid {
		delete(l.backendPIDs, channel)
	}
}

func (l *rawListener) closeConnection(channel string, conn *pgx.Conn) {
	if conn == nil {
		return
	}
	pid := conn.PgConn().PID()
	conn.Close(context.Background())
	l.clearBackendPID(channel, pid)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func validateRawChannelName(channel string) error {
	if len(channel) == 0 || len(channel) > maxPostgresChannelNameBytes {
		return fmt.Errorf("%w: %q", ErrInvalidChannel, channel)
	}
	for i, ch := range channel {
		if ch >= 'a' && ch <= 'z' {
			continue
		}
		if i > 0 && ch >= '0' && ch <= '9' {
			continue
		}
		if i > 0 && ch == '_' {
			continue
		}
		return fmt.Errorf("%w: %q", ErrInvalidChannel, channel)
	}
	return nil
}
