# SDK Contract Fixtures

Canonical server response shapes and shared live seed contracts for cross-SDK parity tests.

This directory is the single source of truth for cross-SDK wire payload fixtures and deterministic seeded-server expectations.

## Fixture Ledger

- `auth_response.json`
- `error_response_numeric_code.json`
- `error_response_string_code.json`
- `list_response.json`
- `list_search_seed_contract.json`
- `magic_link_request_response.json`
- `magic_link_confirm_success_response.json`
- `magic_link_confirm_pending_mfa_response.json`
- `oauth_start_url_cases.json` - table cases for JavaScript SDK OAuth start URL path/query construction.
- `realtime_event.json`
- `search_synonyms_request.json` - canonical PUT envelope for `/api/collections/{table}/synonyms/`.
- `search_synonyms_response.json` - server-normalized GET/PUT response for `/api/collections/{table}/synonyms/`.
- `storage_list_response.json`
- `storage_object.json`
- `webauthn_enroll_begin_response.json` - second-factor WebAuthn MFA enroll begin response from `/api/auth/mfa/webauthn/enroll`; this is the top-level creation-options payload written by `handleWebAuthnEnroll`.
- `webauthn_enroll_confirm_request.json` - canonical second-factor WebAuthn MFA enroll confirm request body for `/api/auth/mfa/webauthn/enroll/confirm`.
- `webauthn_enroll_confirm_response.json` - canonical second-factor WebAuthn MFA enroll confirm response envelope.
- `webauthn_discover_begin_response.json` - discoverable passkey login begin response from `/api/auth/webauthn/login/discover/begin`; preserves the `challenge_id` plus raw `options` envelope without credential descriptors.
- `webauthn_discover_finish_request.json` - canonical discoverable passkey login finish request body for `/api/auth/webauthn/login/discover/finish`; discover finish reuses `auth_response.json`.
- `webauthn_login_begin_response.json` - first-factor passkey login begin response from `/api/auth/webauthn/login/begin`; WebAuthn login finish reuses `auth_response.json`.
- `webauthn_mfa_challenge_response.json` - second-factor WebAuthn MFA challenge response from `/api/auth/mfa/webauthn/challenge`; preserves `challenge_id` plus raw `options`.
- `webauthn_mfa_verify_request.json` - canonical second-factor WebAuthn MFA verify request body for `/api/auth/mfa/webauthn/verify`.
- `webauthn_mfa_verify_response.json` - second-factor WebAuthn MFA verify auth response; shares the same signed-in auth response contract family as `auth_response.json`.

Rules:
- Fixtures in this directory are pure JSON payloads (no metadata wrapper).
- Fixtures preserve canonical wire keys exactly as defined by server contracts (mostly camelCase, with required snake_case fields such as `email_verified`, `created_at`, `updated_at`, and `doc_url` where applicable).
- SDK-specific tests may additionally validate alias support (for example snake_case), but these files are the canonical baseline.
- `magic_link_confirm_pending_mfa_response.json` remains the shared pending-MFA envelope fixture for all MFA factors; WebAuthn MFA does not add a duplicate pending-token fixture.

Capture notes:
- `oauth_start_url_cases.json` freezes JavaScript SDK OAuth start URL construction from `sdk/src/auth.ts` for downstream SDK parity. It is not a server redirect-validation fixture; server validation remains owned by `internal/auth/handler_oauth.go`.
- `webauthn_login_begin_response.json` was captured from the WebAuthn first-factor login begin contract path using the virtual-authenticator ceremony technique from `internal/auth/webauthn_integration_test.go`, then sanitized only for volatile `challenge_id`, `options.challenge`, and credential ID values. Validation command: `go run ./internal/testutil/cmd/testpg -- go tool gotestsum --format testdox -- -tags=integration -count=1 ./internal/auth -run TestWebAuthnFirstFactorLoginBegin_Contract`.
- `webauthn_discover_begin_response.json` and `webauthn_discover_finish_request.json` were captured from `TestWebAuthnDiscoverableLoginBeginFinish_Contract` in `internal/auth/webauthn_integration_test.go` using the live resident-key virtual-authenticator ceremony. The committed payloads came from the sanitized red `assertSDKContractFixture` output, with replacements limited to volatile `challenge_id`, embedded challenge bytes, credential IDs, signatures, and user handles. No separate discover finish response fixture is committed because the real server response stays in the shared signed-in auth envelope family covered by `auth_response.json`. These fixtures let downstream SDKs dispatch discoverable login begin and finish shapes without depending on MFA challenge or email-scoped first-factor fixtures. Validation command: `go run ./internal/testutil/cmd/testpg -- go tool gotestsum --format testdox -- -tags=integration -count=1 ./internal/auth -run TestWebAuthnDiscoverableLoginBeginFinish_Contract`.
- `webauthn_enroll_begin_response.json`, `webauthn_enroll_confirm_request.json`, `webauthn_enroll_confirm_response.json`, `webauthn_mfa_challenge_response.json`, `webauthn_mfa_verify_request.json`, and `webauthn_mfa_verify_response.json` were captured from the WebAuthn second-factor MFA enrollment/challenge/verify contract path using the virtual-authenticator ceremony in `internal/auth/webauthn_integration_test.go`. The committed fixtures preserve deterministic raw contract bytes such as `authenticatorData`, and preserve `clientDataJSON` structure by normalizing only its embedded challenge value; the remaining fixture placeholders are limited to live-random challenge IDs, credential IDs, server user identifiers/user handles, opaque attestation or signature blobs that embed live key material, tokens, and timestamps. Validation command: `go run ./internal/testutil/cmd/testpg -- go tool gotestsum --format testdox -- -tags=integration -count=1 ./internal/auth -run TestWebAuthn`.
- `search_synonyms_request.json` and `search_synonyms_response.json` were captured from an isolated `sdk_contract_synonyms_fixture` collection using the same live round-trip contract asserted by `sdk/src/integration.search_settings.test.ts` and `internal/server/search_synonyms_handler_integration_test.go`: `PUT /api/collections/sdk_contract_synonyms_fixture/synonyms/` with `search_synonyms_request.json`, followed by `GET /api/collections/sdk_contract_synonyms_fixture/synonyms/`.
