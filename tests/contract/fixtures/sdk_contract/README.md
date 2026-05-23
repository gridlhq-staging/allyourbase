# SDK Contract Fixtures

Canonical server response shapes for cross-SDK deserialization parity tests.

This directory is the single source of truth for cross-SDK wire payload fixtures.

## Fixture Ledger

- `auth_response.json`
- `error_response_numeric_code.json`
- `error_response_string_code.json`
- `list_response.json`
- `magic_link_request_response.json`
- `magic_link_confirm_success_response.json`
- `magic_link_confirm_pending_mfa_response.json`
- `realtime_event.json`
- `storage_list_response.json`
- `storage_object.json`

Rules:
- Fixtures in this directory are pure JSON payloads (no metadata wrapper).
- Fixtures preserve canonical wire keys exactly as defined by server contracts (mostly camelCase, with required snake_case fields such as `email_verified`, `created_at`, `updated_at`, and `doc_url` where applicable).
- SDK-specific tests may additionally validate alias support (for example snake_case), but these files are the canonical baseline.
