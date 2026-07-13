-- Migration 181: Drop legacy WebAuthn credential material from _ayb_user_mfa.
--
-- L11 introduced WebAuthn MFA by storing credential material directly on
-- _ayb_user_mfa. Migration 179 moved enabled legacy WebAuthn factors into
-- _ayb_webauthn_credentials and made that child table the runtime source for
-- credential_id, public_key, sign_count, transports, display_name, and
-- last_used_at. Disabled legacy factors are not live authentication material:
-- runtime factor listing and WebAuthn challenge paths require enabled factors,
-- and migration 179 intentionally backfilled only enabled factors.
--
-- The dropped _ayb_user_mfa columns are therefore dead credential-material
-- storage: webauthn_credential_id, webauthn_public_key, webauthn_sign_count,
-- and webauthn_display_name. _ayb_user_mfa.webauthn_session_data stays in
-- place because enrollment still stores active ceremony state there.
-- _ayb_mfa_challenges.webauthn_session_data and
-- _ayb_webauthn_discoverable_challenges.webauthn_session_data also stay in
-- place because current challenge and discoverable-login code reads them.

ALTER TABLE public._ayb_user_mfa
    DROP COLUMN IF EXISTS webauthn_credential_id,
    DROP COLUMN IF EXISTS webauthn_public_key,
    DROP COLUMN IF EXISTS webauthn_sign_count,
    DROP COLUMN IF EXISTS webauthn_display_name;
