# Python SDK

Use the `allyourbase` Python package for async access to auth, records, realtime, and storage.

## Install

Preview — install from source. Registry publishing is tracked for GA. The PyPI name `allyourbase` is already taken by an unrelated package.

```bash
git clone https://github.com/gridlhq-staging/allyourbase.git
cd allyourbase
pip install ./sdk_python
```

Full guide: [docs-site/guide/python-sdk.md](../docs-site/guide/python-sdk.md).

## PostgreSQL RPC

`await client.rpc(function_name, args=None)` returns decoded JSON. A 204 or
empty response returns `None`.

```python
total = await client.rpc(
    "leaderboard_total",
    args={"club_id": "abc123"},
)
```

## Edge functions

`await client.functions.invoke(name, body=..., headers=..., method="POST",
skip_auth=False)` returns the raw response fields `status: int`,
`headers: dict[str, str]`, and `raw_body: bytes`.

```python
response = await client.functions.invoke(
    "send-digest",
    body={"club_id": "abc123"},
)
print(response.status, response.headers, response.raw_body)
```

GraphQL and admin/org typed clients are JS-only scope today. Python
applications can use AYB REST endpoints or custom HTTP calls for those
surfaces.

## Auth helpers

Build an OAuth start URL without mutating the client session:

```python
url = client.auth.get_oauth_start_url(
    "github",
    "csrf-state",
    scopes=["user:email", "read:org"],
    redirect_to="https://app.example.com/oauth/callback",
)
```

Use the raw WebAuthn MFA wire helpers around your own browser authenticator
ceremony. The Python SDK returns and accepts server JSON boundaries; it does not
call `navigator.credentials` or serialize browser authenticator objects.

```python
begin = await client.auth.enroll_webauthn()
attestation_response = {}  # collected in a browser from begin
await client.auth.confirm_webauthn_enrollment(
    "Primary security key",
    attestation_response,
)

challenge = await client.auth.webauthn_challenge(mfa_token)
assertion_response = {}  # collected in a browser from challenge.options
auth = await client.auth.webauthn_verify(
    mfa_token,
    challenge.challenge_id,
    assertion_response,
)
```
