package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrSessionNotFound  = errors.New("session not found")
	ErrSessionForbidden = errors.New("session does not belong to user")
)

// SessionInfo represents a refresh-token-backed auth session.
type SessionInfo struct {
	ID           string    `json:"id"`
	UserAgent    string    `json:"user_agent"`
	IPAddress    string    `json:"ip_address"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	Current      bool      `json:"current"`
}

type sessionIDRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}

// ListSessions lists active sessions for a user and marks the current session.
func (s *Service) ListSessions(ctx context.Context, userID, currentSessionID string) ([]SessionInfo, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id,
		        COALESCE(user_agent, ''),
		        COALESCE(ip_address, ''),
		        created_at,
		        COALESCE(last_active_at, created_at)
		 FROM _ayb_sessions
		 WHERE user_id = $1 AND expires_at > NOW()
		 ORDER BY COALESCE(last_active_at, created_at) DESC, created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]SessionInfo, 0)
	for rows.Next() {
		var session SessionInfo
		if err := rows.Scan(
			&session.ID,
			&session.UserAgent,
			&session.IPAddress,
			&session.CreatedAt,
			&session.LastActiveAt,
		); err != nil {
			return nil, fmt.Errorf("scanning session: %w", err)
		}
		session.Current = currentSessionID != "" && session.ID == currentSessionID
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating sessions: %w", err)
	}

	return sessions, nil
}

// RevokeSession revokes a specific session for a user.
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID string) error {
	if s.hasDurableSessionRevocation() {
		return s.revokeSessionDurably(ctx, userID, sessionID)
	}
	var exists bool
	var owned bool
	err := s.pool.QueryRow(ctx,
		`SELECT
			EXISTS(SELECT 1 FROM _ayb_sessions WHERE id = $1),
			EXISTS(SELECT 1 FROM _ayb_sessions WHERE id = $1 AND user_id = $2)`,
		sessionID, userID,
	).Scan(&exists, &owned)
	if err != nil {
		return fmt.Errorf("checking session ownership: %w", err)
	}
	if !exists {
		return ErrSessionNotFound
	}
	if !owned {
		return ErrSessionForbidden
	}
	if err := s.addSessionToDenyList(ctx, sessionID); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_sessions WHERE id = $1 AND user_id = $2`,
		sessionID, userID,
	); err != nil {
		return fmt.Errorf("revoking session: %w", err)
	}
	return nil
}

// RevokeAllExceptCurrent revokes every session except the current one.
func (s *Service) RevokeAllExceptCurrent(ctx context.Context, userID, currentSessionID string) error {
	if currentSessionID == "" {
		return fmt.Errorf("%w: current session id is required", ErrValidation)
	}
	if s.hasDurableSessionRevocation() {
		expiresAt := s.nowTime().Add(s.tokenDur)
		rows, err := s.pool.Query(ctx,
			`WITH deleted AS (
				DELETE FROM _ayb_sessions
				 WHERE user_id = $1 AND id <> $2
				 RETURNING id
			), persisted AS (
				INSERT INTO _ayb_revoked_sessions (session_id, expires_at)
				SELECT id, $3 FROM deleted
				ON CONFLICT (session_id) DO UPDATE
				SET expires_at = EXCLUDED.expires_at
				RETURNING session_id
			)
			SELECT session_id FROM persisted`,
			userID, currentSessionID, expiresAt,
		)
		if err != nil {
			return fmt.Errorf("revoking sessions: %w", err)
		}
		return s.denyListFromSessionRowsWithExpiry(ctx, rows, expiresAt)
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id FROM _ayb_sessions WHERE user_id = $1 AND id <> $2`,
		userID, currentSessionID,
	)
	if err != nil {
		return fmt.Errorf("listing sessions to revoke: %w", err)
	}
	if err := s.denyListFromSessionRows(ctx, rows); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_sessions WHERE user_id = $1 AND id <> $2`,
		userID, currentSessionID,
	); err != nil {
		return fmt.Errorf("revoking sessions: %w", err)
	}
	return nil
}

// RevokeAllSessions revokes all sessions for a user.
func (s *Service) RevokeAllSessions(ctx context.Context, userID string) error {
	if s.hasDurableSessionRevocation() {
		expiresAt := s.nowTime().Add(s.tokenDur)
		rows, err := s.pool.Query(ctx,
			`WITH deleted AS (
				DELETE FROM _ayb_sessions
				 WHERE user_id = $1
				 RETURNING id
			), persisted AS (
				INSERT INTO _ayb_revoked_sessions (session_id, expires_at)
				SELECT id, $2 FROM deleted
				ON CONFLICT (session_id) DO UPDATE
				SET expires_at = EXCLUDED.expires_at
				RETURNING session_id
			)
			SELECT session_id FROM persisted`,
			userID, expiresAt,
		)
		if err != nil {
			return fmt.Errorf("revoking all sessions: %w", err)
		}
		return s.denyListFromSessionRowsWithExpiry(ctx, rows, expiresAt)
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id FROM _ayb_sessions WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("listing all sessions to revoke: %w", err)
	}
	if err := s.denyListFromSessionRows(ctx, rows); err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM _ayb_sessions WHERE user_id = $1`,
		userID,
	); err != nil {
		return fmt.Errorf("revoking all sessions: %w", err)
	}
	return nil
}

func (s *Service) addSessionToDenyList(ctx context.Context, sessionID string) error {
	if s.denyList == nil || sessionID == "" {
		return nil
	}
	if err := s.denyList.AddContext(ctx, sessionID, s.tokenDur); err != nil {
		return fmt.Errorf("adding revoked session to denylist: %w", err)
	}
	return nil
}

func (s *Service) revokeSessionDurably(ctx context.Context, userID, sessionID string) error {
	expiresAt := s.nowTime().Add(s.tokenDur)
	var revokedSessionID string
	err := s.pool.QueryRow(ctx,
		`WITH deleted AS (
			DELETE FROM _ayb_sessions
			 WHERE id = $1 AND user_id = $2
			 RETURNING id
		), persisted AS (
			INSERT INTO _ayb_revoked_sessions (session_id, expires_at)
			SELECT id, $3 FROM deleted
			ON CONFLICT (session_id) DO UPDATE
			SET expires_at = EXCLUDED.expires_at
			RETURNING session_id
		)
		SELECT session_id FROM persisted`,
		sessionID, userID, expiresAt,
	).Scan(&revokedSessionID)
	if err == nil {
		return s.addSessionToDenyListWithExpiry(ctx, revokedSessionID, expiresAt)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("revoking session: %w", err)
	}

	var exists bool
	err = s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM _ayb_sessions WHERE id = $1)`,
		sessionID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("checking session existence: %w", err)
	}
	if exists {
		return ErrSessionForbidden
	}
	return ErrSessionNotFound
}

func (s *Service) hasDurableSessionRevocation() bool {
	return s != nil && s.pool != nil && s.denyList != nil && s.denyList.currentStore() != nil
}

func (s *Service) addSessionToDenyListWithExpiry(ctx context.Context, sessionID string, expiresAt time.Time) error {
	if s.denyList == nil || sessionID == "" {
		return nil
	}
	if err := s.denyList.applyAndPublish(ctx, sessionID, expiresAt); err != nil {
		return fmt.Errorf("adding revoked session to denylist: %w", err)
	}
	return nil
}

func (s *Service) denyListFromSessionRows(ctx context.Context, rows sessionIDRows) error {
	defer rows.Close()

	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			return fmt.Errorf("scanning revoked session id: %w", err)
		}
		if err := s.addSessionToDenyList(ctx, sessionID); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating revoked session ids: %w", err)
	}
	return nil
}

func (s *Service) denyListFromSessionRowsWithExpiry(ctx context.Context, rows sessionIDRows, expiresAt time.Time) error {
	defer rows.Close()

	for rows.Next() {
		var sessionID string
		if err := rows.Scan(&sessionID); err != nil {
			return fmt.Errorf("scanning revoked session id: %w", err)
		}
		if err := s.addSessionToDenyListWithExpiry(ctx, sessionID, expiresAt); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating revoked session ids: %w", err)
	}
	return nil
}
