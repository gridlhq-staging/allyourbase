from __future__ import annotations

from urllib.parse import parse_qs, quote

import pytest

from allyourbase.client import AYBClient
from allyourbase.errors import AYBError
from allyourbase.types import BatchOperation, ListResponse


async def test_list_no_params(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(
        json={"items": [{"id": "1"}], "page": 1, "perPage": 20, "totalItems": 1, "totalPages": 1}
    )
    client = AYBClient("https://api.example.com")

    result = await client.records.list("posts")

    assert isinstance(result, ListResponse)
    req = httpx_mock.get_request()
    assert req is not None
    assert str(req.url) == "https://api.example.com/api/collections/posts"


async def test_list_all_params(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(
        json={"items": [], "page": 1, "perPage": 10, "totalItems": 0, "totalPages": 0}
    )
    client = AYBClient("https://api.example.com")

    await client.records.list(
        "posts",
        page=1,
        per_page=10,
        sort="-created_at",
        filter="published=true",
        search="hello world",
        fields="id,title",
        expand="author",
        skip_total=True,
    )

    req = httpx_mock.get_request()
    assert req is not None
    params = req.url.params
    assert params["page"] == "1"
    assert params["perPage"] == "10"
    assert params["sort"] == "-created_at"
    assert params["filter"] == "published=true"
    assert params["search"] == "hello world"
    assert params["fields"] == "id,title"
    assert params["expand"] == "author"
    assert params["skipTotal"] == "true"


async def test_list_search_params_match_js_query_contract(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(
        json={"items": [], "page": 1, "perPage": 10, "totalItems": 0, "totalPages": 0}
    )
    client = AYBClient("https://api.example.com")

    await client.records.list(
        "posts",
        search="banan",
        fuzzy=True,
        typo_threshold=0.2,
        highlight=True,
        facets=["category", "author"],
        semantic=True,
        semantic_query="banana semantic",
    )

    req = httpx_mock.get_request()
    assert req is not None
    assert parse_qs(req.url.query.decode(), keep_blank_values=True) == {
        "search": ["banan"],
        "fuzzy": ["true"],
        "typo_threshold": ["0.2"],
        "highlight": ["true"],
        "facets": ["category,author"],
        "semantic": ["true"],
        "semantic_query": ["banana semantic"],
    }


@pytest.mark.parametrize(
    ("kwargs", "expected_query"),
    [
        (
            {"fuzzy": False, "highlight": False, "semantic": False, "facets": []},
            {},
        ),
        (
            {"typo_threshold": 0.0},
            {"typo_threshold": ["0.0"]},
        ),
    ],
)
async def test_list_search_params_omit_false_flags_but_keep_numeric_zero(
    httpx_mock: pytest.fixture,
    kwargs: dict[str, object],
    expected_query: dict[str, list[str]],
) -> None:
    httpx_mock.add_response(
        json={"items": [], "page": 1, "perPage": 10, "totalItems": 0, "totalPages": 0}
    )
    client = AYBClient("https://api.example.com")

    await client.records.list("posts", **kwargs)

    req = httpx_mock.get_request()
    assert req is not None
    assert parse_qs(req.url.query.decode(), keep_blank_values=True) == expected_query


async def test_list_accepts_cursor_envelope(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(
        json={
            "items": [{"id": "rec_1"}],
            "perPage": 10,
            "nextCursor": "cursor_2",
        }
    )
    client = AYBClient("https://api.example.com")

    result = await client.records.list("posts")

    assert isinstance(result, ListResponse)
    assert result.page is None
    assert result.total_items is None
    assert result.total_pages is None
    assert result.next_cursor == "cursor_2"
    assert result.items[0]["id"] == "rec_1"


async def test_list_escapes_collection_path_segment(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(
        json={"items": [], "page": 1, "perPage": 10, "totalItems": 0, "totalPages": 0}
    )
    client = AYBClient("https://api.example.com")
    collection = "posts?admin=true"

    await client.records.list(collection)

    req = httpx_mock.get_request()
    assert req is not None
    assert parse_qs(req.url.query.decode(), keep_blank_values=True) == {}
    assert req.url.raw_path.decode() == f"/api/collections/{quote(collection, safe='')}"


async def test_get_by_id(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json={"id": "rec_1", "title": "Hello"})
    client = AYBClient("https://api.example.com")

    record = await client.records.get("posts", "rec_1")

    assert record["id"] == "rec_1"
    req = httpx_mock.get_request()
    assert req is not None
    assert str(req.url) == "https://api.example.com/api/collections/posts/rec_1"


async def test_get_with_fields_expand(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json={"id": "rec_1"})
    client = AYBClient("https://api.example.com")

    await client.records.get("posts", "rec_1", fields="id", expand="author")

    req = httpx_mock.get_request()
    assert req is not None
    assert req.url.params["fields"] == "id"
    assert req.url.params["expand"] == "author"


async def test_create(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=201, json={"id": "rec_1", "title": "Hello"})
    client = AYBClient("https://api.example.com")

    created = await client.records.create("posts", {"title": "Hello"})

    assert created["id"] == "rec_1"
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert req.content == b'{"title":"Hello"}'


async def test_update(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json={"id": "rec_1", "title": "Updated"})
    client = AYBClient("https://api.example.com")

    updated = await client.records.update("posts", "rec_1", {"title": "Updated"})

    assert updated["title"] == "Updated"
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "PATCH"
    assert req.content == b'{"title":"Updated"}'


async def test_delete_returns_none(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=204)
    client = AYBClient("https://api.example.com")

    result = await client.records.delete("posts", "rec_1")

    assert result is None
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "DELETE"


async def test_batch(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(
        json=[
            {"index": 0, "status": 201, "body": {"id": "rec_1"}},
            {"index": 1, "status": 204},
        ]
    )
    client = AYBClient("https://api.example.com")

    ops = [
        BatchOperation(method="create", body={"title": "A"}),
        BatchOperation(method="delete", id="rec_1"),
    ]
    collection = "posts?admin=true"
    result = await client.records.batch(collection, ops)

    assert len(result) == 2
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert parse_qs(req.url.query.decode(), keep_blank_values=True) == {}
    assert req.url.raw_path.decode() == f"/api/collections/{quote(collection, safe='')}/batch"
    assert req.content == (
        b'{"operations":[{"method":"create","body":{"title":"A"}},'
        b'{"method":"delete","id":"rec_1"}]}'
    )


async def test_records_error_propagates(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=404, json={"message": "Not found"})
    client = AYBClient("https://api.example.com")

    with pytest.raises(AYBError) as exc:
        await client.records.get("posts", "missing")

    assert exc.value.status == 404
