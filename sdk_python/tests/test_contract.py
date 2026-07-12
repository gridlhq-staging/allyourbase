from __future__ import annotations

import json
from pathlib import Path

import httpx
import pytest

from allyourbase.client import AYBClient
from allyourbase.errors import AYBError
from allyourbase.types import (
    AuthResponse,
    BatchResult,
    ListResponse,
    MagicLinkConfirmResponse,
    MagicLinkRequestResponse,
    RealtimeEvent,
    SearchSynonymGroup,
    SearchSynonymsRequest,
    SearchSynonymsResponse,
    StorageListResponse,
    StorageObject,
    User,
    WebAuthnLoginBeginResponse,
)

_CONTRACT_FIXTURE_DIR = (
    Path(__file__).resolve().parents[2] / "tests" / "contract" / "fixtures" / "sdk_contract"
)
_PARITY_FIXTURE_DIR = (
    Path(__file__).resolve().parents[2] / "tests" / "contract" / "fixtures" / "sdk_parity"
)
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
_SEARCH_SYNONYMS_REQUEST_FIXTURE = json.loads(
    (_CONTRACT_FIXTURE_DIR / "search_synonyms_request.json").read_text()
)
_SEARCH_SYNONYMS_RESPONSE_FIXTURE = json.loads(
    (_CONTRACT_FIXTURE_DIR / "search_synonyms_response.json").read_text()
)
_ANONYMOUS_FIXTURE = json.loads((_PARITY_FIXTURE_DIR / "anonymous.json").read_text())
_LINK_EMAIL_FIXTURE = json.loads((_PARITY_FIXTURE_DIR / "link_email.json").read_text())


def test_auth_response_matches_server_shape() -> None:
    raw = {
        "token": "jwt_stage3",
        "refreshToken": "refresh_stage3",
        "user": {
            "id": "usr_1",
            "email": "dev@allyourbase.io",
            "email_verified": True,
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": None,
        },
    }
    auth = AuthResponse.model_validate(raw)
    assert auth.token == "jwt_stage3"
    assert auth.refresh_token == "refresh_stage3"
    assert auth.user.email_verified is True
    assert auth.user.created_at == "2026-01-01T00:00:00Z"
    assert auth.user.updated_at is None


def test_user_minimal_fields() -> None:
    user = User.model_validate({"id": "usr_2", "email": "bob@example.com"})
    assert user.id == "usr_2"
    assert user.email == "bob@example.com"


def test_magic_link_request_response_matches_canonical_shape() -> None:
    response = MagicLinkRequestResponse.model_validate(_MAGIC_LINK_REQUEST_RESPONSE_FIXTURE)
    assert response.message == "If an account exists, a magic link has been sent."


def test_magic_link_confirm_response_parses_success_and_pending_mfa_payloads() -> None:
    success_response = MagicLinkConfirmResponse.model_validate(_MAGIC_LINK_CONFIRM_SUCCESS_FIXTURE)
    pending_response = MagicLinkConfirmResponse.model_validate(
        _MAGIC_LINK_CONFIRM_PENDING_MFA_FIXTURE
    )

    assert success_response.is_pending_mfa is False
    assert success_response.user is not None
    assert success_response.user.email == "magic@allyourbase.io"
    assert success_response.user.email_verified is True
    assert success_response.user.created_at == "2026-05-01T12:00:00Z"
    assert success_response.user.updated_at is None

    assert pending_response.is_pending_mfa is True
    assert pending_response.mfa_pending is True
    assert pending_response.mfa_token == "mfa_pending_token_stage1"


def test_webauthn_login_begin_response_matches_canonical_shape() -> None:
    response = WebAuthnLoginBeginResponse.model_validate(_WEBAUTHN_LOGIN_BEGIN_RESPONSE_FIXTURE)

    assert response.challenge_id == "webauthn_challenge_fixture"
    assert response.options["challenge"] == "webauthn_login_begin_challenge"
    assert response.options["rpId"] == "127.0.0.1"
    assert response.options["timeout"] == 300000
    assert response.options["allowCredentials"][0] == {
        "id": "webauthn_login_begin_credential_a",
        "type": "public-key",
    }


def test_webauthn_mfa_enroll_begin_response_matches_canonical_shape() -> None:
    from allyourbase import types as allyourbase_types

    response_type = getattr(allyourbase_types, "WebAuthnEnrollBeginResponse")
    response = response_type.model_validate(_WEBAUTHN_ENROLL_BEGIN_RESPONSE_FIXTURE)

    assert response.challenge == "webauthn_enroll_begin_challenge"
    assert response.rp == {"id": "127.0.0.1", "name": "Allyourbase"}
    assert response.user == {
        "id": "webauthn_enroll_user_id",
        "name": "webauthn-e2e@example.com",
        "displayName": "webauthn-e2e@example.com",
    }
    assert response.pub_key_cred_params == [
        {"type": "public-key", "alg": -7},
        {"type": "public-key", "alg": -35},
        {"type": "public-key", "alg": -36},
        {"type": "public-key", "alg": -257},
        {"type": "public-key", "alg": -258},
        {"type": "public-key", "alg": -259},
        {"type": "public-key", "alg": -37},
        {"type": "public-key", "alg": -38},
        {"type": "public-key", "alg": -39},
        {"type": "public-key", "alg": -8},
    ]
    assert response.timeout == 300000
    assert response.attestation == "none"
    assert response.exclude_credentials == []


def test_webauthn_mfa_challenge_response_matches_canonical_shape() -> None:
    from allyourbase import types as allyourbase_types

    response_type = getattr(allyourbase_types, "WebAuthnMFAChallengeResponse")
    response = response_type.model_validate(_WEBAUTHN_MFA_CHALLENGE_RESPONSE_FIXTURE)

    assert response.challenge_id == "webauthn_mfa_challenge_fixture"
    assert response.options == {
        "allowCredentials": [
            {
                "id": "webauthn_mfa_credential_a",
                "type": "public-key",
            }
        ],
        "challenge": "webauthn_mfa_challenge",
        "rpId": "127.0.0.1",
        "timeout": 300000,
    }
    assert response.options["allowCredentials"] == [
        {"id": "webauthn_mfa_credential_a", "type": "public-key"}
    ]


def test_webauthn_mfa_enroll_confirm_fixtures_match_canonical_envelopes() -> None:
    from allyourbase import types as allyourbase_types

    request_type = getattr(allyourbase_types, "WebAuthnEnrollConfirmRequest")
    response_type = getattr(allyourbase_types, "WebAuthnEnrollConfirmResponse")
    request = request_type.model_validate(_WEBAUTHN_ENROLL_CONFIRM_REQUEST_FIXTURE)
    response = response_type.model_validate(_WEBAUTHN_ENROLL_CONFIRM_RESPONSE_FIXTURE)

    assert request.display_name == "Primary security key"
    assert request.attestation_response == {
        "clientExtensionResults": {},
        "id": "webauthn_enroll_credential",
        "rawId": "webauthn_enroll_credential",
        "response": {
            "attestationObject": "webauthn_enroll_attestation_object",
            "clientDataJSON": (
                "eyJ0eXBlIjoid2ViYXV0aG4uY3JlYXRlIiwiY2hhbGxlbmdlIjoid2ViYXV0"
                "aG5fZW5yb2xsX2JlZ2luX2NoYWxsZW5nZSIsIm9yaWdpbiI6Imh0dHA6Ly8x"
                "MjcuMC4wLjE6ODA5MCJ9"
            ),
            "transports": ["internal"],
        },
        "type": "public-key",
    }
    assert response.message == "WebAuthn MFA enrollment confirmed"


def test_webauthn_mfa_verify_fixtures_match_canonical_envelopes() -> None:
    from allyourbase import types as allyourbase_types

    request_type = getattr(allyourbase_types, "WebAuthnMFAVerifyRequest")
    request = request_type.model_validate(_WEBAUTHN_MFA_VERIFY_REQUEST_FIXTURE)
    response = AuthResponse.model_validate(_WEBAUTHN_MFA_VERIFY_RESPONSE_FIXTURE)

    assert request.challenge_id == "webauthn_mfa_challenge_fixture"
    assert request.assertion_response == {
        "clientExtensionResults": {},
        "id": "webauthn_mfa_credential",
        "rawId": "webauthn_mfa_credential",
        "response": {
            "authenticatorData": "EsoXtJryKJQ28wPgFmAwoh5SXSZuIJJnQzgBqP1AcaAFAAAAAQ",
            "clientDataJSON": (
                "eyJ0eXBlIjoid2ViYXV0aG4uZ2V0IiwiY2hhbGxlbmdlIjoid2ViYXV0"
                "aG5fbWZhX2NoYWxsZW5nZSIsIm9yaWdpbiI6Imh0dHA6Ly8xMjcuMC4wLjE6"
                "ODA5MCJ9"
            ),
            "signature": "webauthn_mfa_signature",
            "userHandle": "webauthn_mfa_user_handle",
        },
        "type": "public-key",
    }
    assert response.token == "jwt_webauthn_mfa"
    assert response.refresh_token == "refresh_webauthn_mfa"
    assert response.user.id == "usr_webauthn_mfa"
    assert response.user.email == "webauthn-e2e@example.com"
    assert response.user.created_at == "2026-07-11T00:00:00Z"
    assert response.user.updated_at == "2026-07-11T00:00:00Z"


def test_user_model_accepts_snake_case_and_camelcase_auth_fields() -> None:
    anonymous_user = User.model_validate(_ANONYMOUS_FIXTURE["response"]["user"])
    linked_user = User.model_validate(_LINK_EMAIL_FIXTURE["response"]["user"])

    assert anonymous_user.is_anonymous is True
    assert anonymous_user.created_at is not None
    assert anonymous_user.updated_at is not None
    assert linked_user.linked_at is not None
    assert linked_user.email_verified is None


async def test_ayberror_parses_server_error_response_doc_url() -> None:
    responses = [
        httpx.Response(
            status_code=403,
            json={
                "code": 403,
                "message": "forbidden",
                "data": {"resource": "posts"},
                "doc_url": "https://allyourbase.io/docs/errors#forbidden",
            },
        )
    ]

    def handler(_: httpx.Request) -> httpx.Response:
        return responses.pop(0)

    http_client = httpx.AsyncClient(transport=httpx.MockTransport(handler))
    client = AYBClient("https://api.example.com", http_client=http_client)
    try:
        with pytest.raises(AYBError) as exc:
            await client._request("/api/auth/login", method="POST")

        assert exc.value.status == 403
        assert exc.value.code == "403"
        assert exc.value.message == "forbidden"
        assert exc.value.doc_url == "https://allyourbase.io/docs/errors#forbidden"
        assert exc.value.data == {"resource": "posts"}
    finally:
        await client.close()


async def test_ayberror_parses_server_error_response_string_code() -> None:
    responses = [
        httpx.Response(
            status_code=400,
            json={
                "code": "auth/missing-refresh-token",
                "message": "Missing refresh token",
                "data": {"detail": "refresh token not available"},
            },
        )
    ]

    def handler(_: httpx.Request) -> httpx.Response:
        return responses.pop(0)

    http_client = httpx.AsyncClient(transport=httpx.MockTransport(handler))
    client = AYBClient("https://api.example.com", http_client=http_client)
    try:
        with pytest.raises(AYBError) as exc:
            await client._request("/api/auth/refresh", method="POST")

        assert exc.value.status == 400
        assert exc.value.code == "auth/missing-refresh-token"
        assert exc.value.message == "Missing refresh token"
        assert exc.value.data == {"detail": "refresh token not available"}
    finally:
        await client.close()


def test_list_response_shape() -> None:
    response = ListResponse.model_validate(
        {
            "page": 1,
            "perPage": 2,
            "totalItems": 2,
            "totalPages": 1,
            "items": [{"id": "rec_1", "title": "First"}, {"id": "rec_2", "title": "Second"}],
        }
    )
    assert response.total_items == 2
    assert response.items[0]["title"] == "First"
    assert response.items[1]["title"] == "Second"


def test_search_synonyms_request_fixture_matches_canonical_shape() -> None:
    response = SearchSynonymsRequest.model_validate(_SEARCH_SYNONYMS_REQUEST_FIXTURE)

    assert isinstance(response.groups[0], SearchSynonymGroup)
    assert [group.terms for group in response.groups] == [
        ["scifi", "science fiction"],
        ["nyc", "new york"],
    ]


def test_search_synonyms_response_fixture_matches_canonical_shape() -> None:
    response = SearchSynonymsResponse.model_validate(_SEARCH_SYNONYMS_RESPONSE_FIXTURE)

    assert isinstance(response.groups[0], SearchSynonymGroup)
    assert [group.terms for group in response.groups] == [
        ["new york", "nyc"],
        ["science fiction", "scifi"],
    ]


def test_batch_result_with_and_without_body() -> None:
    a = BatchResult.model_validate({"index": 0, "status": 201, "body": {"id": "x"}})
    b = BatchResult.model_validate({"index": 2, "status": 204})
    assert a.body == {"id": "x"}
    assert b.body is None


def test_storage_object_with_and_without_userid() -> None:
    with_user = StorageObject.model_validate(
        {
            "id": "file_abc123",
            "bucket": "uploads",
            "name": "document.pdf",
            "size": 1024,
            "contentType": "application/pdf",
            "userId": "usr_1",
            "createdAt": "2026-01-01T00:00:00Z",
            "updatedAt": "2026-01-02T12:30:00Z",
        }
    )
    assert with_user.user_id == "usr_1"
    assert with_user.content_type == "application/pdf"
    assert with_user.updated_at == "2026-01-02T12:30:00Z"


def test_storage_list_response_shape_with_nullable_fields() -> None:
    response = StorageListResponse.model_validate(
        {
            "items": [
                {
                    "id": "file_1",
                    "bucket": "uploads",
                    "name": "doc1.pdf",
                    "size": 1024,
                    "contentType": "application/pdf",
                    "userId": "usr_1",
                    "createdAt": "2026-01-01T00:00:00Z",
                    "updatedAt": None,
                },
                {
                    "id": "file_2",
                    "bucket": "uploads",
                    "name": "image.png",
                    "size": 2048,
                    "contentType": "image/png",
                    "userId": None,
                    "createdAt": "2026-01-02T00:00:00Z",
                    "updatedAt": None,
                },
            ],
            "totalItems": 2,
        }
    )
    assert response.total_items == 2
    assert response.items[0].user_id == "usr_1"
    assert response.items[0].updated_at is None
    assert response.items[1].user_id is None
    assert response.items[1].updated_at is None


def test_realtime_event_shape() -> None:
    event = RealtimeEvent.model_validate(
        {
            "action": "UPDATE",
            "table": "posts",
            "record": {"id": "rec_1", "title": "after"},
            "oldRecord": {"id": "rec_1", "title": "before"},
        }
    )
    assert event.record["id"] == "rec_1"
    assert event.old_record == {"id": "rec_1", "title": "before"}


async def test_geojson_round_trip_plain_dict() -> None:
    polygon = {
        "type": "Polygon",
        "coordinates": [
            [[-73.9, 40.7], [-73.8, 40.7], [-73.8, 40.8], [-73.9, 40.8], [-73.9, 40.7]]
        ],
    }
    requests: list[httpx.Request] = []
    responses = [
        httpx.Response(
            status_code=201, json={"id": "zone_1", "name": "Manhattan", "geometry": polygon}
        ),
        httpx.Response(
            status_code=200, json={"id": "zone_1", "name": "Manhattan", "geometry": polygon}
        ),
    ]

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return responses.pop(0)

    http_client = httpx.AsyncClient(transport=httpx.MockTransport(handler))
    client = AYBClient("https://api.example.com", http_client=http_client)
    try:
        created = await client.records.create("zones", {"name": "Manhattan", "geometry": polygon})
        fetched = await client.records.get("zones", "zone_1")

        assert created["geometry"] == polygon
        assert fetched["geometry"] == polygon

        create_req = requests[0]
        assert create_req.content == (
            b'{"name":"Manhattan","geometry":{"type":"Polygon","coordinates":'
            b'[[[-73.9,40.7],[-73.8,40.7],[-73.8,40.8],[-73.9,40.8],[-73.9,40.7]]]}}'
        )
    finally:
        await client.close()
