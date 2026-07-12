-- Migration 179: Store each WebAuthn credential in a child table.
-- _ayb_user_mfa remains the canonical factor owner; legacy WebAuthn columns
-- stay in place for compatibility while enrollment/verification are migrated.

CREATE TABLE IF NOT EXISTS public._ayb_webauthn_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    factor_id UUID NOT NULL REFERENCES public._ayb_user_mfa(id) ON DELETE CASCADE,
    credential_id BYTEA NOT NULL,
    public_key BYTEA NOT NULL,
    transports TEXT[] NOT NULL DEFAULT '{}',
    sign_count BIGINT NOT NULL DEFAULT 0,
    display_name TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    CONSTRAINT uq_ayb_webauthn_credentials_credential_id UNIQUE (credential_id)
);

CREATE INDEX IF NOT EXISTS idx_ayb_webauthn_credentials_factor_id
    ON public._ayb_webauthn_credentials (factor_id);

INSERT INTO public._ayb_webauthn_credentials (
    factor_id,
    credential_id,
    public_key,
    sign_count,
    display_name
)
SELECT
    id,
    webauthn_credential_id,
    webauthn_public_key,
    COALESCE(webauthn_sign_count, 0),
    COALESCE(webauthn_display_name, '')
FROM public._ayb_user_mfa
WHERE method = 'webauthn'
  AND enabled = true
  AND webauthn_credential_id IS NOT NULL
  AND webauthn_public_key IS NOT NULL
ON CONFLICT (credential_id) DO NOTHING;
