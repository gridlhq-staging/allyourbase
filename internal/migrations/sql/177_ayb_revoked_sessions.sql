-- Durable revoked auth sessions.
-- Runtime ownership remains in internal/auth; this table stores only the
-- revocation facts needed for later denylist reload and cleanup.

CREATE TABLE IF NOT EXISTS _ayb_revoked_sessions (
    session_id TEXT PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ayb_revoked_sessions_expires_at
    ON _ayb_revoked_sessions (expires_at);
