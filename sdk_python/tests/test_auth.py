from __future__ import annotations

import json
from pathlib import Path
from urllib.parse import parse_qsl, urlsplit

import pytest

from allyourbase.client import AYBClient
from allyourbase.errors import AYBError
from allyourbase.types import (
    AuthResponse,
    MagicLinkConfirmResponse,
    MagicLinkRequestResponse,
    User,
    WebAuthnLoginBeginResponse,
)

_FIXTURE_DIR = (
    Path(__file__).resolve().parents[2] / "tests" / "contract" / "fixtures" / "sdk_parity"
)
_CONTRACT_FIXTURE_DIR = (
    Path(__file__).resolve().parents[2] / "tests" / "contract" / "fixtures" / "sdk_contract"
)
_ANONYMOUS_FIXTURE = json.loads((_FIXTURE_DIR / "anonymous.json").read_text())
_MAGIC_LINK_REQUEST_RESPONSE_FIXTURE = json.loads(
    (_CONTRACT_FIXTURE_DIR / "magic_link_request_response.json").read_text()
)
_MAGIC_LINK_CONFIRM_SUCCESS_FIXTURE = json.loads(
    (_CONTRACT_FIXTURE_DIR / "magic_link_confirm_success_response.json").read_text()
)
_MAGIC_LINK_CONFIRM_PENDING_MFA_FIXTURE = json.loads(
    (_CONTRACT_FIXTURE_DIR / "magic_link_confirm_pending_mfa_response.json").read_text()
)
_WEBAUTHN_LOGIN_BEGIN_RESPONSE_FIXTURE = json.loads(
    (_CONTRACT_FIXTURE_DIR / "webauthn_login_begin_response.json").read_text()
)
_WEBAUTHN_ENROLL_BEGIN_RESPONSE_FIXTURE = json.loads(
    (_CONTRACT_FIXTURE_DIR / "webauthn_enroll_begin_response.json").read_text()
)
_WEBAUTHN_ENROLL_CONFIRM_REQUEST_FIXTURE = json.loads(
    (_CONTRACT_FIXTURE_DIR / "webauthn_enroll_confirm_request.json").read_text()
)
_WEBAUTHN_ENROLL_CONFIRM_RESPONSE_FIXTURE = json.loads(
    (_CONTRACT_FIXTURE_DIR / "webauthn_enroll_confirm_response.json").read_text()
)
_WEBAUTHN_MFA_CHALLENGE_RESPONSE_FIXTURE = json.loads(
    (_CONTRACT_FIXTURE_DIR / "webauthn_mfa_challenge_response.json").read_text()
)
_WEBAUTHN_MFA_VERIFY_REQUEST_FIXTURE = json.loads(
    (_CONTRACT_FIXTURE_DIR / "webauthn_mfa_verify_request.json").read_text()
)
_WEBAUTHN_MFA_VERIFY_RESPONSE_FIXTURE = json.loads(
    (_CONTRACT_FIXTURE_DIR / "webauthn_mfa_verify_response.json").read_text()
)
_AUTH_RESPONSE_FIXTURE = json.loads((_CONTRACT_FIXTURE_DIR / "auth_response.json").read_text())
_OAUTH_START_URL_CASES_FIXTURE = json.loads(
    (_CONTRACT_FIXTURE_DIR / "oauth_start_url_cases.json").read_text()
)
_LINK_EMAIL_FIXTURE = json.loads((_FIXTURE_DIR / "link_email.json").read_text())


def _auth_payload(token: str = "tok", refresh: str = "ref") -> dict[str, object]:
    return {
        "token": token,
        "refreshToken": refresh,
        "user": {
            "id": "usr_1",
            "email": "alice@example.com",
            "createdAt": "2026-02-22T10:00:00Z",
            "updatedAt": "2026-02-22T11:00:00Z",
        },
    }


async def test_register_sends_request_stores_tokens_emits_event(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json=_auth_payload("tok_reg", "ref_reg"))
    client = AYBClient("https://api.example.com")

    events: list[tuple[str, dict[str, str] | None]] = []
    client.on_auth_state_change(lambda e, s: events.append((e, s)))

    result = await client.auth.register("alice@example.com", "secret")

    assert isinstance(result, AuthResponse)
    assert client.token == "tok_reg"
    assert client.refresh_token == "ref_reg"
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert str(req.url) == "https://api.example.com/api/auth/register"
    assert req.content == b'{"email":"alice@example.com","password":"secret"}'
    assert events == [
        ("SIGNED_IN", {"token": "tok_reg", "refresh_token": "ref_reg"}),
    ]


async def test_login_sends_request_stores_tokens_emits_event(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json=_auth_payload("tok_login", "ref_login"))
    client = AYBClient("https://api.example.com")

    events: list[str] = []
    client.on_auth_state_change(lambda e, _: events.append(e))

    await client.auth.login("alice@example.com", "secret")

    assert client.token == "tok_login"
    assert client.refresh_token == "ref_login"
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert str(req.url) == "https://api.example.com/api/auth/login"
    assert events == ["SIGNED_IN"]


async def test_begin_webauthn_login_sends_email_without_mutating_tokens(
    httpx_mock: pytest.fixture,
) -> None:
    httpx_mock.add_response(json=_WEBAUTHN_LOGIN_BEGIN_RESPONSE_FIXTURE)
    client = AYBClient("https://api.example.com")

    result = await client.auth.begin_webauthn_login("dev@allyourbase.io")

    assert isinstance(result, WebAuthnLoginBeginResponse)
    assert result.challenge_id == "webauthn_challenge_fixture"
    assert result.options == _WEBAUTHN_LOGIN_BEGIN_RESPONSE_FIXTURE["options"]
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert str(req.url) == "https://api.example.com/api/auth/webauthn/login/begin"
    assert req.content == b'{"email":"dev@allyourbase.io"}'
    assert client.token is None
    assert client.refresh_token is None


async def test_finish_webauthn_login_sends_assertion_stores_tokens_emits_event(
    httpx_mock: pytest.fixture,
) -> None:
    httpx_mock.add_response(json=_AUTH_RESPONSE_FIXTURE)
    client = AYBClient("https://api.example.com")
    assertion = {
        "id": "webauthn_login_begin_credential_a",
        "rawId": "d2ViYXV0aG5fbG9naW5fYmVnaW5fY3JlZGVudGlhbF9h",
        "type": "public-key",
        "response": {
            "clientDataJSON": "Y2xpZW50LWRhdGE",
            "authenticatorData": "YXV0aC1kYXRh",
            "signature": "c2lnbmF0dXJl",
            "userHandle": None,
        },
        "clientExtensionResults": {},
    }
    events: list[tuple[str, dict[str, str] | None]] = []
    client.on_auth_state_change(lambda e, s: events.append((e, s)))

    result = await client.auth.finish_webauthn_login(
        "webauthn_challenge_fixture",
        assertion,
    )

    assert isinstance(result, AuthResponse)
    assert result.token == "jwt_stage3"
    assert result.refresh_token == "refresh_stage3"
    assert result.user.email == "dev@allyourbase.io"
    assert client.token == "jwt_stage3"
    assert client.refresh_token == "refresh_stage3"
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert str(req.url) == "https://api.example.com/api/auth/webauthn/login/finish"
    assert json.loads(req.content) == {
        "challenge_id": "webauthn_challenge_fixture",
        "assertion_response": assertion,
    }
    assert events == [
        ("SIGNED_IN", {"token": "jwt_stage3", "refresh_token": "refresh_stage3"}),
    ]


async def test_enroll_webauthn_uses_session_bearer_and_returns_creation_options(
    httpx_mock: pytest.fixture,
) -> None:
    client = AYBClient("https://api.example.com")
    client.set_tokens("session_token", "session_refresh")

    enroll_webauthn = getattr(client.auth, "enroll_webauthn")
    httpx_mock.add_response(json=_WEBAUTHN_ENROLL_BEGIN_RESPONSE_FIXTURE)
    result = await enroll_webauthn()

    assert result.challenge == "webauthn_enroll_begin_challenge"
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert str(req.url) == "https://api.example.com/api/auth/mfa/webauthn/enroll"
    assert req.headers["authorization"] == "Bearer session_token"
    assert req.content == b""


async def test_confirm_webauthn_enrollment_uses_session_bearer_and_fixture_body(
    httpx_mock: pytest.fixture,
) -> None:
    client = AYBClient("https://api.example.com")
    client.set_tokens("session_token", "session_refresh")

    confirm_webauthn_enrollment = getattr(client.auth, "confirm_webauthn_enrollment")
    httpx_mock.add_response(json=_WEBAUTHN_ENROLL_CONFIRM_RESPONSE_FIXTURE)
    result = await confirm_webauthn_enrollment(
        _WEBAUTHN_ENROLL_CONFIRM_REQUEST_FIXTURE["display_name"],
        _WEBAUTHN_ENROLL_CONFIRM_REQUEST_FIXTURE["attestation_response"],
    )

    assert result.message == "WebAuthn MFA enrollment confirmed"
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert str(req.url) == "https://api.example.com/api/auth/mfa/webauthn/enroll/confirm"
    assert req.headers["authorization"] == "Bearer session_token"
    assert json.loads(req.content) == _WEBAUTHN_ENROLL_CONFIRM_REQUEST_FIXTURE


async def test_webauthn_challenge_uses_explicit_mfa_token_bearer(
    httpx_mock: pytest.fixture,
) -> None:
    client = AYBClient("https://api.example.com")
    client.set_tokens("session_token", "session_refresh")

    webauthn_challenge = getattr(client.auth, "webauthn_challenge")
    httpx_mock.add_response(json=_WEBAUTHN_MFA_CHALLENGE_RESPONSE_FIXTURE)
    result = await webauthn_challenge("pending_mfa_token")

    assert result.challenge_id == "webauthn_mfa_challenge_fixture"
    assert result.options == _WEBAUTHN_MFA_CHALLENGE_RESPONSE_FIXTURE["options"]
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert str(req.url) == "https://api.example.com/api/auth/mfa/webauthn/challenge"
    assert req.headers["authorization"] == "Bearer pending_mfa_token"
    assert req.content == b""


async def test_webauthn_verify_uses_explicit_mfa_token_fixture_body_and_stores_tokens(
    httpx_mock: pytest.fixture,
) -> None:
    client = AYBClient("https://api.example.com")
    client.set_tokens("session_token", "session_refresh")
    events: list[tuple[str, dict[str, str] | None]] = []
    client.on_auth_state_change(lambda e, s: events.append((e, s)))

    webauthn_verify = getattr(client.auth, "webauthn_verify")
    httpx_mock.add_response(json=_WEBAUTHN_MFA_VERIFY_RESPONSE_FIXTURE)
    result = await webauthn_verify(
        "pending_mfa_token",
        _WEBAUTHN_MFA_VERIFY_REQUEST_FIXTURE["challenge_id"],
        _WEBAUTHN_MFA_VERIFY_REQUEST_FIXTURE["assertion_response"],
    )

    assert isinstance(result, AuthResponse)
    assert result.token == "jwt_webauthn_mfa"
    assert result.refresh_token == "refresh_webauthn_mfa"
    assert client.token == "jwt_webauthn_mfa"
    assert client.refresh_token == "refresh_webauthn_mfa"
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert str(req.url) == "https://api.example.com/api/auth/mfa/webauthn/verify"
    assert req.headers["authorization"] == "Bearer pending_mfa_token"
    assert json.loads(req.content) == _WEBAUTHN_MFA_VERIFY_REQUEST_FIXTURE
    assert events == [
        ("SIGNED_IN", {"token": "jwt_webauthn_mfa", "refresh_token": "refresh_webauthn_mfa"})
    ]


async def test_delete_webauthn_uses_session_bearer(httpx_mock: pytest.fixture) -> None:
    client = AYBClient("https://api.example.com")
    client.set_tokens("session_token", "session_refresh")

    delete_webauthn = getattr(client.auth, "delete_webauthn")
    httpx_mock.add_response(status_code=204)
    await delete_webauthn()

    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "DELETE"
    assert str(req.url) == "https://api.example.com/api/auth/mfa/webauthn/"
    assert req.headers["authorization"] == "Bearer session_token"
    assert req.content == b""


@pytest.mark.parametrize("fixture_case", _OAUTH_START_URL_CASES_FIXTURE)
def test_get_oauth_start_url_matches_canonical_fixture_cases(
    fixture_case: dict[str, object],
) -> None:
    client = AYBClient(str(fixture_case["base_url"]))

    get_oauth_start_url = getattr(client.auth, "get_oauth_start_url")
    actual_url = get_oauth_start_url(
        fixture_case["provider"],
        fixture_case["state"],
        scopes=fixture_case.get("scopes"),
        redirect_to=fixture_case.get("redirect_to"),
    )

    actual = urlsplit(actual_url)
    expected = urlsplit(f"{fixture_case['base_url']}{fixture_case['expected_path_query']}")
    actual_query = parse_qsl(actual.query, keep_blank_values=True)
    expected_query = parse_qsl(expected.query, keep_blank_values=True)

    assert f"{actual.path}?{actual.query}" == fixture_case["expected_path_query"]
    assert actual.path == expected.path
    assert [key for key, _ in actual_query] == [key for key, _ in expected_query]
    assert dict(actual_query)["state"] == fixture_case["state"]

    if "scopes" in fixture_case:
        assert "scopes=" in actual.query
        assert dict(actual_query)["scopes"] == ",".join(fixture_case["scopes"])
    else:
        assert "scopes" not in dict(actual_query)

    if "redirect_to" in fixture_case:
        assert "redirect_to=" in actual.query
        assert dict(actual_query)["redirect_to"] == fixture_case["redirect_to"]
    else:
        assert "redirect_to" not in dict(actual_query)


async def test_sign_in_anonymously_stores_tokens_and_emits_event(
    httpx_mock: pytest.fixture,
) -> None:
    httpx_mock.add_response(json=_ANONYMOUS_FIXTURE["response"], status_code=201)
    client = AYBClient("https://api.example.com")

    events: list[str] = []
    client.on_auth_state_change(lambda e, _: events.append(e))

    result = await client.auth.sign_in_anonymously()

    assert isinstance(result, AuthResponse)
    assert result.user.is_anonymous is True
    assert client.token == result.token
    assert client.refresh_token == result.refresh_token
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert str(req.url) == "https://api.example.com/api/auth/anonymous"
    assert req.content == b"{}"
    assert events == ["SIGNED_IN"]


async def test_request_magic_link_uses_shared_contract_fixture(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json=_MAGIC_LINK_REQUEST_RESPONSE_FIXTURE)
    client = AYBClient("https://api.example.com")

    result = await client.auth.request_magic_link("fixture@example.com")

    assert isinstance(result, MagicLinkRequestResponse)
    assert result.message == _MAGIC_LINK_REQUEST_RESPONSE_FIXTURE["message"]
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert str(req.url) == "https://api.example.com/api/auth/magic-link"
    assert req.content == b'{"email":"fixture@example.com"}'
    assert client.token is None
    assert client.refresh_token is None


async def test_confirm_magic_link_stores_tokens_for_authenticated_response(
    httpx_mock: pytest.fixture,
) -> None:
    httpx_mock.add_response(json=_MAGIC_LINK_CONFIRM_SUCCESS_FIXTURE)
    client = AYBClient("https://api.example.com")

    events: list[str] = []
    client.on_auth_state_change(lambda e, _: events.append(e))

    result = await client.auth.confirm_magic_link("sdk-parity-magic-token")

    assert isinstance(result, MagicLinkConfirmResponse)
    assert result.is_pending_mfa is False
    assert result.user is not None
    assert result.user.email == "magic@allyourbase.io"
    assert result.user.email_verified is True
    assert result.user.created_at == "2026-05-01T12:00:00Z"
    assert result.user.updated_at is None
    assert client.token == result.token
    assert client.refresh_token == result.refresh_token
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert str(req.url) == "https://api.example.com/api/auth/magic-link/confirm"
    assert req.content == b'{"token":"sdk-parity-magic-token"}'
    assert events == ["SIGNED_IN"]


async def test_confirm_magic_link_returns_pending_mfa_without_mutating_tokens(
    httpx_mock: pytest.fixture,
) -> None:
    httpx_mock.add_response(json=_MAGIC_LINK_CONFIRM_PENDING_MFA_FIXTURE)
    client = AYBClient("https://api.example.com")
    client.set_tokens("existing_token", "existing_refresh")
    events: list[str] = []
    client.on_auth_state_change(lambda e, _: events.append(e))

    result = await client.auth.confirm_magic_link("pending-token")

    assert result.is_pending_mfa is True
    assert result.mfa_token == "mfa_pending_token_stage1"
    assert client.token == "existing_token"
    assert client.refresh_token == "existing_refresh"
    assert events == []


async def test_link_email_uses_shared_contract_fixture_and_auth_header(
    httpx_mock: pytest.fixture,
) -> None:
    httpx_mock.add_response(json=_LINK_EMAIL_FIXTURE["response"])
    client = AYBClient("https://api.example.com")
    client.set_tokens("anon_token", "anon_refresh")

    events: list[str] = []
    client.on_auth_state_change(lambda e, _: events.append(e))

    result = await client.auth.link_email("upgraded@example.com", "LinkedPass123!")

    assert isinstance(result, AuthResponse)
    assert result.user.email == "upgraded@example.com"
    assert result.user.is_anonymous is None
    assert result.user.linked_at is not None
    assert client.token == result.token
    assert client.refresh_token == result.refresh_token
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert str(req.url) == "https://api.example.com/api/auth/link/email"
    assert req.headers["authorization"] == "Bearer anon_token"
    assert req.content == b'{"email":"upgraded@example.com","password":"LinkedPass123!"}'
    assert events == ["SIGNED_IN"]


async def test_me_gets_user_with_auth_header(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(
        json={
            "id": "usr_1",
            "email": "alice@example.com",
            "createdAt": "2026-02-22T10:00:00Z",
            "updatedAt": "2026-02-22T11:00:00Z",
        }
    )
    client = AYBClient("https://api.example.com")
    client.set_tokens("tok", "ref")

    user = await client.auth.me()

    assert isinstance(user, User)
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "GET"
    assert req.headers["authorization"] == "Bearer tok"


async def test_refresh_sends_refresh_token_updates_state_emits_event(
    httpx_mock: pytest.fixture,
) -> None:
    httpx_mock.add_response(json=_auth_payload("tok_new", "ref_new"))
    client = AYBClient("https://api.example.com")
    client.set_tokens("tok_old", "ref_old")

    events: list[tuple[str, dict[str, str] | None]] = []
    client.on_auth_state_change(lambda e, s: events.append((e, s)))

    await client.auth.refresh()

    req = httpx_mock.get_request()
    assert req is not None
    assert req.content == b'{"refreshToken":"ref_old"}'
    assert client.token == "tok_new"
    assert client.refresh_token == "ref_new"
    assert events == [
        ("TOKEN_REFRESHED", {"token": "tok_new", "refresh_token": "ref_new"})
    ]


async def test_logout_sends_refresh_token_clears_tokens_emits_signed_out(
    httpx_mock: pytest.fixture,
) -> None:
    httpx_mock.add_response(status_code=204)
    client = AYBClient("https://api.example.com")
    client.set_tokens("tok", "ref")

    events: list[tuple[str, dict[str, str] | None]] = []
    client.on_auth_state_change(lambda e, s: events.append((e, s)))

    await client.auth.logout()

    req = httpx_mock.get_request()
    assert req is not None
    assert req.content == b'{"refreshToken":"ref"}'
    assert client.token is None
    assert client.refresh_token is None
    assert events == [("SIGNED_OUT", None)]


async def test_delete_account_clears_tokens_emits_signed_out(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=204)
    client = AYBClient("https://api.example.com")
    client.set_tokens("tok", "ref")

    events: list[str] = []
    client.on_auth_state_change(lambda e, _: events.append(e))

    await client.auth.delete_account()

    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "DELETE"
    assert str(req.url) == "https://api.example.com/api/auth/me"
    assert client.token is None
    assert client.refresh_token is None
    assert events == ["SIGNED_OUT"]


async def test_request_password_reset_payload(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=204)
    client = AYBClient("https://api.example.com")

    await client.auth.request_password_reset("alice@example.com")

    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert str(req.url) == "https://api.example.com/api/auth/password-reset"
    assert req.content == b'{"email":"alice@example.com"}'


async def test_confirm_password_reset_payload(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=204)
    client = AYBClient("https://api.example.com")

    await client.auth.confirm_password_reset("reset-token", "new-password")

    req = httpx_mock.get_request()
    assert req is not None
    assert str(req.url) == "https://api.example.com/api/auth/password-reset/confirm"
    assert req.content == b'{"token":"reset-token","password":"new-password"}'


async def test_verify_email_payload(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=204)
    client = AYBClient("https://api.example.com")

    await client.auth.verify_email("verify-token")

    req = httpx_mock.get_request()
    assert req is not None
    assert str(req.url) == "https://api.example.com/api/auth/verify"
    assert req.content == b'{"token":"verify-token"}'


async def test_resend_verification_sends_auth(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=204)
    client = AYBClient("https://api.example.com")
    client.set_tokens("tok", "ref")

    await client.auth.resend_verification()

    req = httpx_mock.get_request()
    assert req is not None
    assert str(req.url) == "https://api.example.com/api/auth/verify/resend"
    assert req.method == "POST"
    assert req.headers["authorization"] == "Bearer tok"


async def test_auth_error_propagates_as_ayberror(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=401, json={"message": "Invalid email or password"})
    client = AYBClient("https://api.example.com")

    with pytest.raises(AYBError) as exc:
        await client.auth.login("alice@example.com", "wrong")

    assert exc.value.status == 401
    assert exc.value.message == "Invalid email or password"
