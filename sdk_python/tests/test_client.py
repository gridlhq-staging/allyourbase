"""Tests for AYBClient core."""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from allyourbase.client import AYBClient
from allyourbase.errors import AYBError


def test_base_url_normalizes_trailing_slashes() -> None:
    client = AYBClient("https://api.example.com///")
    assert client.base_url == "https://api.example.com"


def test_sub_clients_accessible() -> None:
    client = AYBClient("https://api.example.com")
    assert client.auth is not None
    assert client.records is not None
    assert client.storage is not None
    assert client.realtime is not None


def test_set_tokens_and_clear_tokens() -> None:
    client = AYBClient("https://api.example.com")
    client.set_tokens("tok_123", "refresh_123")
    assert client.token == "tok_123"
    assert client.refresh_token == "refresh_123"

    client.clear_tokens()
    assert client.token is None
    assert client.refresh_token is None


def test_set_api_key_stores_as_token_clears_refresh() -> None:
    client = AYBClient("https://api.example.com")
    client.set_tokens("jwt_123", "refresh_123")

    client.set_api_key("ayb_key_123")
    assert client.token == "ayb_key_123"
    assert client.refresh_token is None


def test_clear_api_key() -> None:
    client = AYBClient("https://api.example.com")
    client.set_api_key("ayb_key_123")

    client.clear_api_key()
    assert client.token is None
    assert client.refresh_token is None


async def test_request_injects_auth_header(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json={"ok": True})
    client = AYBClient("https://api.example.com")
    client.set_tokens("jwt_abc", "refresh_abc")

    resp = await client._request("/api/me")
    assert resp.json() == {"ok": True}

    request = httpx_mock.get_request()
    assert request is not None
    assert request.headers["authorization"] == "Bearer jwt_abc"


async def test_request_skips_auth_with_skip_auth(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json={"ok": True})
    client = AYBClient("https://api.example.com")
    client.set_tokens("jwt_abc", "refresh_abc")

    await client._request("/api/public", skip_auth=True)

    request = httpx_mock.get_request()
    assert request is not None
    assert "authorization" not in request.headers


async def test_request_raises_ayb_error_with_normalized_fields(
    httpx_mock: pytest.fixture,
) -> None:
    httpx_mock.add_response(
        status_code=409,
        json={
            "message": "unique violation",
            "code": "db/unique",
            "data": {"users_email_key": {"code": "unique_violation"}},
            "doc_url": "https://allyourbase.io/guide/errors#db-unique",
        },
    )
    client = AYBClient("https://api.example.com")

    with pytest.raises(AYBError) as exc_info:
        await client._request("/api/fail")

    err = exc_info.value
    assert err.status == 409
    assert err.message == "unique violation"
    assert err.code == "db/unique"
    assert err.data == {"users_email_key": {"code": "unique_violation"}}
    assert err.doc_url == "https://allyourbase.io/guide/errors#db-unique"


async def test_request_raises_ayb_error_with_status_text_for_invalid_json(
    httpx_mock: pytest.fixture,
) -> None:
    httpx_mock.add_response(status_code=502, text="<!doctype html>gateway")
    client = AYBClient("https://api.example.com")

    with pytest.raises(AYBError) as exc_info:
        await client._request("/api/fail")

    err = exc_info.value
    assert err.status == 502


async def test_request_returns_none_for_204(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=204)
    client = AYBClient("https://api.example.com")

    resp = await client._request("/api/items/1", method="DELETE")
    assert resp is None


async def test_request_preserves_204_response_when_requested(
    httpx_mock: pytest.fixture,
) -> None:
    httpx_mock.add_response(status_code=204, headers={"x-edge-version": "v1"})
    client = AYBClient("https://api.example.com")

    resp = await client._request(
        "/api/items/1",
        method="DELETE",
        return_response_on_204=True,
    )
    assert resp is not None
    assert resp.status_code == 204
    assert resp.headers["x-edge-version"] == "v1"


async def test_context_manager(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json={"ok": True})
    async with AYBClient("https://api.example.com") as client:
        resp = await client._request("/api/test")
        assert resp.json() == {"ok": True}


async def test_rpc_posts_json_body(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json=42)
    client = AYBClient("https://api.example.com")

    result = await client.rpc("get_total", args={"user_id": "abc"})
    assert result == 42

    request = httpx_mock.get_request()
    assert request is not None
    assert request.method == "POST"
    assert str(request.url) == "https://api.example.com/api/rpc/get_total"
    assert request.headers["content-type"] == "application/json"


async def test_rpc_omits_body_when_args_none(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json="ok")
    client = AYBClient("https://api.example.com")

    result = await client.rpc("no_args_fn")
    assert result == "ok"

    request = httpx_mock.get_request()
    assert request is not None
    assert request.method == "POST"
    assert request.content == b""


async def test_rpc_omits_body_when_args_empty(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json="ok")
    client = AYBClient("https://api.example.com")

    await client.rpc("no_args_fn", args={})

    request = httpx_mock.get_request()
    assert request is not None
    assert request.content == b""


# --- Edge function invoke (client.functions.invoke) ---

_CONTRACT_FIXTURE_DIR = (
    Path(__file__).resolve().parents[2] / "tests" / "contract" / "fixtures" / "sdk_contract"
)
_EDGE_REQUEST_FIXTURE = json.loads(
    (_CONTRACT_FIXTURE_DIR / "edge_invoke_request.json").read_text()
)
# Compact wire bytes the server emits, reconstructed from the on-disk fixtures.
_EDGE_REQUEST_BYTES = json.dumps(_EDGE_REQUEST_FIXTURE, separators=(",", ":")).encode()
_EDGE_RESPONSE_BYTES = json.dumps(
    json.loads((_CONTRACT_FIXTURE_DIR / "edge_invoke_response.json").read_text()),
    separators=(",", ":"),
).encode()


async def test_edge_invoke_sends_post_with_bearer_and_exact_bytes(
    httpx_mock: pytest.fixture,
) -> None:
    httpx_mock.add_response(
        content=_EDGE_RESPONSE_BYTES,
        headers={"content-type": "application/json"},
    )
    client = AYBClient("https://api.example.com")
    client.set_tokens("jwt_edge", "refresh_edge")

    result = await client.functions.invoke(
        "sdk-contract-echo",
        body=_EDGE_REQUEST_FIXTURE,
        headers={"X-Trace": "abc"},
    )

    request = httpx_mock.get_request()
    assert request is not None
    assert request.method == "POST"
    assert str(request.url) == "https://api.example.com/functions/v1/sdk-contract-echo"
    assert request.headers["authorization"] == "Bearer jwt_edge"
    assert request.headers["x-trace"] == "abc"
    assert request.headers["content-type"] == "application/json"
    assert request.content == _EDGE_REQUEST_BYTES

    assert result.status == 200
    assert result.raw_body == _EDGE_RESPONSE_BYTES
    assert json.loads(result.raw_body) == {
        "message": "sdk-live",
        "method": "POST",
        "specimen": "sdk-contract-echo",
    }
    assert result.headers["content-type"] == "application/json"


async def test_edge_invoke_encodes_name_single_segment(
    httpx_mock: pytest.fixture,
) -> None:
    httpx_mock.add_response(content=b"{}")
    client = AYBClient("https://api.example.com")

    await client.functions.invoke("weird/name with spaces")

    request = httpx_mock.get_request()
    assert request is not None
    assert (
        str(request.url)
        == "https://api.example.com/functions/v1/weird%2Fname%20with%20spaces"
    )


async def test_edge_invoke_omits_bearer_when_skip_auth(
    httpx_mock: pytest.fixture,
) -> None:
    httpx_mock.add_response(content=b"{}")
    client = AYBClient("https://api.example.com")
    client.set_tokens("jwt_edge", "refresh_edge")

    await client.functions.invoke(
        "sdk-contract-echo", body=_EDGE_REQUEST_FIXTURE, skip_auth=True
    )

    request = httpx_mock.get_request()
    assert request is not None
    assert "authorization" not in request.headers


async def test_edge_invoke_returns_empty_raw_body_on_204(
    httpx_mock: pytest.fixture,
) -> None:
    httpx_mock.add_response(status_code=204, headers={"x-edge-version": "v1"})
    client = AYBClient("https://api.example.com")

    result = await client.functions.invoke("sdk-contract-echo")
    assert result.status == 204
    assert result.headers["x-edge-version"] == "v1"
    assert result.raw_body == b""


async def test_edge_invoke_preserves_normalized_ayb_error(
    httpx_mock: pytest.fixture,
) -> None:
    httpx_mock.add_response(
        status_code=500,
        json={"message": "boom", "code": "edge_error"},
    )
    client = AYBClient("https://api.example.com")

    with pytest.raises(AYBError) as exc_info:
        await client.functions.invoke("sdk-contract-echo", body=_EDGE_REQUEST_FIXTURE)

    err = exc_info.value
    assert err.status == 500
    assert err.message == "boom"
    assert err.code == "edge_error"
