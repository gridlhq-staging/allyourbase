from __future__ import annotations

import json
import os
import uuid
from pathlib import Path

import pytest

from allyourbase import AYBClient, AYBError

_BASE_LIVE_ENV_MISSING = (
    not os.environ.get("AYB_TEST_URL") or not os.environ.get("AYB_TEST_COLLECTION")
)
_ADMIN_LIVE_ENV_MISSING = _BASE_LIVE_ENV_MISSING or (
    not os.environ.get("AYB_ADMIN_TOKEN") and not os.environ.get("AYB_ADMIN_PASSWORD")
)
_CONTRACT_FIXTURE_DIR = (
    Path(__file__).resolve().parents[2] / "tests" / "contract" / "fixtures" / "sdk_contract"
)
_LIST_SEARCH_CONTRACT = json.loads(
    (_CONTRACT_FIXTURE_DIR / "list_search_seed_contract.json").read_text()
)


def _live_base_url() -> str:
    return os.environ["AYB_TEST_URL"]


def _live_collection() -> str:
    return os.environ["AYB_TEST_COLLECTION"]


async def _admin_token(client: AYBClient) -> str:
    if token := os.environ.get("AYB_ADMIN_TOKEN"):
        return token

    resp = await client._request(
        "/api/admin/auth",
        method="POST",
        json={"password": os.environ["AYB_ADMIN_PASSWORD"]},
        skip_auth=True,
    )
    if resp is None:
        raise RuntimeError("Expected response body for admin auth")

    token = str(resp.json().get("token", ""))
    assert token
    return token


@pytest.mark.skipif(
    _BASE_LIVE_ENV_MISSING,
    reason="AYB_TEST_URL or AYB_TEST_COLLECTION is not set",
)
@pytest.mark.asyncio
async def test_e2e_contract_live_server() -> None:
    base_url = _live_base_url()
    collection = _live_collection()
    email = f"sdkpy-{uuid.uuid4().hex[:12]}@example.com"
    password = "P@ssw0rd!123"

    client = AYBClient(base_url)
    try:
        auth = await client.auth.register(email, password)
        assert auth.token
        assert auth.refresh_token
        assert auth.user.email == email

        # Keep the shared facet contract stable by creating the e2e row in its
        # own category instead of incrementing the seeded "docs" bucket.
        created = await client.records.create(
            collection,
            {"title": "sdk python e2e", "category": "sdk"},
        )
        record_id = str(created.get("id"))
        assert record_id

        listed = await client.records.list(collection)
        assert any(str(item.get("id")) == record_id for item in listed.items)

        highlighted = await client.records.list(
            collection,
            search=str(_LIST_SEARCH_CONTRACT["highlightSearch"]),
            highlight=True,
        )
        assert highlighted.items[0]["title"] == _LIST_SEARCH_CONTRACT["highlightedTitle"]
        assert highlighted.items[0].get("_highlight")

        fuzzy = await client.records.list(
            collection,
            search=str(_LIST_SEARCH_CONTRACT["fuzzySearch"]),
            fuzzy=True,
            typo_threshold=float(_LIST_SEARCH_CONTRACT["fuzzyTypoThreshold"]),
        )
        assert fuzzy.items[0]["title"] == _LIST_SEARCH_CONTRACT["fuzzyMatchTitle"]

        facet_column = str(_LIST_SEARCH_CONTRACT["facetColumn"])
        faceted = await client.records.list(collection, facets=[facet_column])
        assert faceted.facets is not None
        assert {
            bucket.value: bucket.count for bucket in faceted.facets[facet_column]
        } == {
            **_LIST_SEARCH_CONTRACT["expectedFacetCounts"],
            "sdk": 1,
        }

        fetched = await client.records.get(collection, record_id)
        assert str(fetched.get("id")) == record_id

        updated = await client.records.update(
            collection,
            record_id,
            {"title": "sdk python e2e updated"},
        )
        assert str(updated.get("id")) == record_id

        await client.records.delete(collection, record_id)

        with pytest.raises(AYBError) as exc:
            await client.records.get(collection, record_id)
        assert exc.value.status == 404

        await client.auth.logout()
        assert client.token is None
    finally:
        await client.close()


@pytest.mark.skipif(
    _BASE_LIVE_ENV_MISSING,
    reason="AYB_TEST_URL or AYB_TEST_COLLECTION is not set",
)
@pytest.mark.asyncio
async def test_e2e_webauthn_begin_live_server() -> None:
    email = f"sdkpy-passkey-{uuid.uuid4().hex[:12]}@example.com"
    client = AYBClient(_live_base_url())
    try:
        result = await client.auth.begin_webauthn_login(email)

        assert result.challenge_id
        assert result.options["challenge"]
        assert result.options["allowCredentials"]
    finally:
        await client.close()


@pytest.mark.skipif(
    _ADMIN_LIVE_ENV_MISSING,
    reason=(
        "AYB_TEST_URL, AYB_TEST_COLLECTION, and either AYB_ADMIN_TOKEN or "
        "AYB_ADMIN_PASSWORD must be set"
    ),
)
@pytest.mark.asyncio
async def test_e2e_synonyms_round_trip_live_server() -> None:
    collection = _live_collection()
    client = AYBClient(_live_base_url())
    try:
        client.set_api_key(await _admin_token(client))
        existing = await client.records.get_synonyms(collection)
        original_groups = [{"terms": group.terms} for group in existing.groups]
        test_groups = [
            {"terms": [f" SDKPY-{uuid.uuid4().hex[:12]} ", "Python SDK E2E"]},
            {"terms": ["Live Synonym Parity", "live synonym contract"]},
        ]
        expected_groups = sorted(
            sorted(term.strip().lower() for term in group["terms"])
            for group in test_groups
        )

        try:
            updated = await client.records.set_synonyms(collection, test_groups)
            assert [group.terms for group in updated.groups] == expected_groups

            fetched = await client.records.get_synonyms(collection)
            assert [group.terms for group in fetched.groups] == expected_groups
        finally:
            await client.records.set_synonyms(collection, original_groups)
    finally:
        await client.close()
