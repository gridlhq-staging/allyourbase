-- Migration 172: Add explicit scope discriminator for MFA challenge rows.
-- Prevents first-factor and MFA-pending WebAuthn ceremonies from being cross-redeemed.

ALTER TABLE _ayb_mfa_challenges
    ADD COLUMN IF NOT EXISTS challenge_scope TEXT NOT NULL DEFAULT 'mfa';
