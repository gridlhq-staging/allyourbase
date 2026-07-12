-- Migration 180: Add WebAuthn discoverable login challenge sessions.
-- Discoverable login begins without a known user, so it cannot use
-- _ayb_mfa_challenges.factor_id.

CREATE TABLE IF NOT EXISTS public._ayb_webauthn_discoverable_challenges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ip_address INET,
    webauthn_session_data BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    verified_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '5 minutes')
);
