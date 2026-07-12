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
