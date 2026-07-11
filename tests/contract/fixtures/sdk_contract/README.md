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
- `realtime_event.json`
- `search_synonyms_request.json` - canonical PUT envelope for `/api/collections/{table}/synonyms/`.
- `search_synonyms_response.json` - server-normalized GET/PUT response for `/api/collections/{table}/synonyms/`.
- `storage_list_response.json`
- `storage_object.json`
- `webauthn_login_begin_response.json` - first-factor passkey login begin response from `/api/auth/webauthn/login/begin`; WebAuthn login finish reuses `auth_response.json`.

Rules:
- Fixtures in this directory are pure JSON payloads (no metadata wrapper).
- Fixtures preserve canonical wire keys exactly as defined by server contracts (mostly camelCase, with required snake_case fields such as `email_verified`, `created_at`, `updated_at`, and `doc_url` where applicable).
- SDK-specific tests may additionally validate alias support (for example snake_case), but these files are the canonical baseline.

Capture notes:
- `webauthn_login_begin_response.json` was captured from the WebAuthn first-factor login begin contract path using the virtual-authenticator ceremony technique from `internal/auth/webauthn_integration_test.go`, then sanitized only for volatile `challenge_id`, `options.challenge`, and credential ID values. Validation command: `go run ./internal/testutil/cmd/testpg -- go tool gotestsum --format testdox -- -tags=integration -count=1 ./internal/auth -run TestWebAuthnFirstFactorLoginBegin_Contract`.
- `search_synonyms_request.json` and `search_synonyms_response.json` were captured from an isolated `sdk_contract_synonyms_fixture` collection using the same live round-trip contract asserted by `sdk/src/integration.search_settings.test.ts` and `internal/server/search_synonyms_handler_integration_test.go`: `PUT /api/collections/sdk_contract_synonyms_fixture/synonyms/` with `search_synonyms_request.json`, followed by `GET /api/collections/sdk_contract_synonyms_fixture/synonyms/`.
