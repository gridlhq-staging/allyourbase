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
    StorageListResponse,
    StorageObject,
    User,
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
    pending_response = MagicLinkConfirmResponse.model_validate(_MAGIC_LINK_CONFIRM_PENDING_MFA_FIXTURE)

    assert success_response.is_pending_mfa is False
    assert success_response.user is not None
    assert success_response.user.email == "magic@allyourbase.io"
    assert success_response.user.email_verified is True
    assert success_response.user.created_at == "2026-05-01T12:00:00Z"
    assert success_response.user.updated_at is None

    assert pending_response.is_pending_mfa is True
    assert pending_response.mfa_pending is True
    assert pending_response.mfa_token == "mfa_pending_token_stage1"


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
        "coordinates": [[[-73.9, 40.7], [-73.8, 40.7], [-73.8, 40.8], [-73.9, 40.8], [-73.9, 40.7]]],
    }
    requests: list[httpx.Request] = []
    responses = [
        httpx.Response(status_code=201, json={"id": "zone_1", "name": "Manhattan", "geometry": polygon}),
        httpx.Response(status_code=200, json={"id": "zone_1", "name": "Manhattan", "geometry": polygon}),
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
