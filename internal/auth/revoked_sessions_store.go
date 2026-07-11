package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrRevokedSessionStoreNotConfigured = errors.New("revoked session store is not configured")

// RevokedSession is a durable revoked-session record that is still active.
type RevokedSession struct {
	SessionID string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// RevokedSessionStore persists revoked session ids for later denylist recovery.
type RevokedSessionStore struct {
	pool *pgxpool.Pool
}

// NewRevokedSessionStore creates a RevokedSessionStore.
func NewRevokedSessionStore(pool *pgxpool.Pool) *RevokedSessionStore {
	return &RevokedSessionStore{pool: pool}
}

// Upsert stores or extends a revoked session until its absolute token expiry.
func (s *RevokedSessionStore) Upsert(ctx context.Context, sessionID string, expiresAt time.Time) error {
	if s == nil || s.pool == nil {
		return ErrRevokedSessionStoreNotConfigured
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("%w: session_id is required", ErrValidation)
	}
	if expiresAt.IsZero() {
		return fmt.Errorf("%w: expires_at is required", ErrValidation)
	}

	_, err := s.pool.Exec(ctx,
		`INSERT INTO _ayb_revoked_sessions (session_id, expires_at)
		 VALUES ($1, $2)
		 ON CONFLICT (session_id) DO UPDATE
		 SET expires_at = EXCLUDED.expires_at`,
		sessionID, expiresAt,
	)
	if err != nil {
		return fmt.Errorf("upserting revoked session: %w", err)
	}
	return nil
}

// LoadActive returns revoked sessions that have not reached their token expiry.
func (s *RevokedSessionStore) LoadActive(ctx context.Context) ([]RevokedSession, error) {
	if s == nil || s.pool == nil {
		return nil, ErrRevokedSessionStoreNotConfigured
	}
	rows, err := s.pool.Query(ctx,
		`SELECT session_id, expires_at, created_at
		   FROM _ayb_revoked_sessions
		  WHERE expires_at > NOW()
		  ORDER BY session_id`,
	)
	if err != nil {
		return nil, fmt.Errorf("loading active revoked sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]RevokedSession, 0)
	for rows.Next() {
		var session RevokedSession
		if err := rows.Scan(&session.SessionID, &session.ExpiresAt, &session.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning revoked session: %w", err)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating revoked sessions: %w", err)
	}
	return sessions, nil
}

// CleanupExpired removes revoked sessions whose token expiry has passed.
func (s *RevokedSessionStore) CleanupExpired(ctx context.Context) (int64, error) {
	if s == nil || s.pool == nil {
		return 0, ErrRevokedSessionStoreNotConfigured
	}
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_revoked_sessions
		  WHERE expires_at <= NOW()`,
	)
	if err != nil {
		return 0, fmt.Errorf("cleaning up expired revoked sessions: %w", err)
	}
	return tag.RowsAffected(), nil
}
