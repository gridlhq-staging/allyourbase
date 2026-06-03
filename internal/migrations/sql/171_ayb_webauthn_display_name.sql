-- Migration 171: Store a human-readable label for WebAuthn MFA factors.
-- This lets the dashboard render the same distinctive passkey name that the
-- user entered during enrollment, which the browser E2E asserts end to end.

ALTER TABLE _ayb_user_mfa
    ADD COLUMN IF NOT EXISTS webauthn_display_name TEXT;
