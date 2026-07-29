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
- `rpc_request.json` - frozen request body for `POST /api/rpc/sdk_contract_add` (named int args `{"a","b"}`).
- `rpc_response.json` - captured OUT-param record response from `POST /api/rpc/sdk_contract_add`; keys are alphabetical (`specimen` before `sum`) because the server marshals the record as a Go map.
- `edge_invoke_request.json` - frozen request body for `POST /functions/v1/sdk-contract-echo` (`{"message":"sdk-live"}`).
- `edge_invoke_response.json` - captured response from `POST /functions/v1/sdk-contract-echo`; echoes the caller's `message` and stamps `method` + the `specimen` marker.
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

Org-admin wire-contract fixtures (captured by `TestAdminOrgsSDKContractFixtures` through the live `/api/admin/orgs*` router). Request fixtures freeze the exact JSON body sent to the server; response fixtures freeze the decoded body. UUID linkages (`id`, `orgId`, `teamId`, `userId`, `tenantId`, `parentOrgId`) and `createdAt`/`updatedAt` timestamps are sanitized to stable placeholders (`<id>`, `<orgId>`, `<timestamp>`, ...); names, slugs, plan/role/status values, list `items` envelopes, and counts are preserved verbatim:

- `org_admin_org_create_request.json` / `org_admin_org_create_response.json` - `POST /api/admin/orgs` create-org body and `201` organization.
- `org_admin_org_list_response.json` - `GET /api/admin/orgs` `{items:[...]}` envelope.
- `org_admin_org_get_response.json` - `GET /api/admin/orgs/{orgId}` organization plus `childOrgCount`/`teamCount`/`tenantCount`.
- `org_admin_org_update_request.json` / `org_admin_org_update_response.json` - `PUT /api/admin/orgs/{orgId}` update body and `200` organization.
- `org_admin_org_usage_response.json` - `GET /api/admin/orgs/{orgId}/usage` summary (`period`, `data`, `totals`, `tenantCount`).
- `org_admin_org_audit_response.json` - `GET /api/admin/orgs/{orgId}/audit` `{items,count,limit,offset}` envelope.
- `org_admin_team_create_request.json` / `org_admin_team_create_response.json` - `POST /api/admin/orgs/{orgId}/teams` body and `201` team.
- `org_admin_team_list_response.json` - `GET .../teams` `{items:[...]}` envelope.
- `org_admin_team_get_response.json` - `GET .../teams/{teamId}` team.
- `org_admin_team_update_request.json` / `org_admin_team_update_response.json` - `PUT .../teams/{teamId}` update body and `200` team.
- `org_admin_org_member_add_request.json` / `org_admin_org_member_add_response.json` - `POST .../members` body and `201` org membership.
- `org_admin_org_member_list_response.json` - `GET .../members` `{items:[...]}` envelope.
- `org_admin_org_member_role_update_request.json` / `org_admin_org_member_role_update_response.json` - `PUT .../members/{userId}/role` body and `200` org membership.
- `org_admin_team_member_add_request.json` / `org_admin_team_member_add_response.json` - `POST .../teams/{teamId}/members` body and `201` team membership.
- `org_admin_team_member_list_response.json` - `GET .../teams/{teamId}/members` `{items:[...]}` envelope.
- `org_admin_team_member_role_update_request.json` / `org_admin_team_member_role_update_response.json` - `PUT .../teams/{teamId}/members/{userId}/role` body and `200` team membership.
- `org_admin_tenant_assign_request.json` / `org_admin_tenant_assign_response.json` - `POST .../tenants` body and `200 {status:"assigned"}`.
- `org_admin_tenant_list_response.json` - `GET .../tenants` `{items:[...]}` envelope of assigned tenants.

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
- `org_admin_*.json` were captured by `TestAdminOrgsSDKContractFixtures` in `internal/server/admin_orgs_fixture_capture_integration_test.go`, which drives one dependency-ordered admin-token flow through the real `server.Router` `/api/admin/orgs*` endpoints against the freshly reset/migrated `sharedPG` managed Postgres. The test never calls `os.WriteFile`; each committed payload is the sanitized red-run output pasted in by hand. Sanitization normalizes only the volatile UUID linkages (`id`, `orgId`, `teamId`, `userId`, `tenantId`, `parentOrgId`), the `createdAt`/`updatedAt` timestamps, and any usage `date` volatility to stable placeholders, while preserving names, slugs, `planTier`/`role`/`state` values, `items` list envelopes, and numeric counts. No token fixture exists because the admin bearer token lives only in the request `Authorization` header and is never serialized into a body. No fixture exists for the `204 No Content` delete/unassign/removal responses (org/team deletion, tenant unassignment, org/team member removal) because they carry no body; the capture test still asserts their exact `204` status. Regeneration / drift-check command: `go run ./internal/testutil/cmd/testpg -- go tool gotestsum --format testdox -- -tags=integration -count=1 ./internal/server -run '^TestAdminOrgsSDKContractFixtures$'`.
- `search_synonyms_request.json` and `search_synonyms_response.json` were captured from an isolated `sdk_contract_synonyms_fixture` collection using the same live round-trip contract asserted by `sdk/src/integration.search_settings.test.ts` and `internal/server/search_synonyms_handler_integration_test.go`: `PUT /api/collections/sdk_contract_synonyms_fixture/synonyms/` with `search_synonyms_request.json`, followed by `GET /api/collections/sdk_contract_synonyms_fixture/synonyms/`.
- `rpc_request.json`, `rpc_response.json`, `edge_invoke_request.json`, and `edge_invoke_response.json` are the deterministic SDK RPC + edge live specimens seeded by `scripts/sdk_live_proof_seed.sh` (owner of the `sdk_contract_add` RPC function and the `sdk-contract-echo` edge function; payloads built by `build_rpc_function_sql` / `build_edge_echo_deploy_json`). Request fixtures freeze the exact bodies sent; response fixtures are the real captured bytes (bodies only — no headers/tokens/URLs/timestamps). Edge invoke needs **no auth** (the function is deployed `public:true`); the RPC probe used the admin bearer. The RPC response keys are alphabetical (`specimen` before `sum`) because the server marshals the OUT-param record as a Go `map[string]any`. Capture command (run from the repo root): `AYB_STORAGE_ENABLED=true AYB_AUTH_ENABLED=true AYB_AUTH_ANONYMOUS_AUTH_ENABLED=true bash scripts/run-with-ayb.sh 'bash scripts/sdk_live_proof_seed.sh && curl -fsS -H "Authorization: Bearer $(<~/.ayb/admin-token)" -H "Content-Type: application/json" -d "{\"a\":2,\"b\":3}" "$AYB_BASE_URL/api/rpc/sdk_contract_add"; curl -fsS -H "Content-Type: application/json" -d "{\"message\":\"sdk-live\"}" "$AYB_BASE_URL/functions/v1/sdk-contract-echo"'`. These are the deterministic contract source consumed by Stages 3-6 SDK RPC/edge parity work.
