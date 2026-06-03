-- Migration 170: Add WebAuthn MFA columns
-- Extends _ayb_user_mfa for passkey/security-key credential storage
-- and _ayb_mfa_challenges for login ceremony session data.

ALTER TABLE _ayb_user_mfa
    ADD COLUMN IF NOT EXISTS webauthn_credential_id BYTEA,
    ADD COLUMN IF NOT EXISTS webauthn_public_key BYTEA,
    ADD COLUMN IF NOT EXISTS webauthn_sign_count BIGINT DEFAULT 0,
    ADD COLUMN IF NOT EXISTS webauthn_session_data BYTEA;

ALTER TABLE _ayb_mfa_challenges
    ADD COLUMN IF NOT EXISTS webauthn_session_data BYTEA;
