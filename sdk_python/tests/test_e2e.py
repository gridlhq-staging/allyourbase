from __future__ import annotations

import json
import os
import uuid
from pathlib import Path

import pytest

from allyourbase import AYBClient, AYBError

_CONTRACT_FIXTURE_DIR = (
    Path(__file__).resolve().parents[2] / "tests" / "contract" / "fixtures" / "sdk_contract"
)
_LIST_SEARCH_CONTRACT = json.loads(
    (_CONTRACT_FIXTURE_DIR / "list_search_seed_contract.json").read_text()
)


@pytest.mark.skipif(
    not os.environ.get("AYB_TEST_URL") or not os.environ.get("AYB_TEST_COLLECTION"),
    reason="AYB_TEST_URL or AYB_TEST_COLLECTION is not set",
)
@pytest.mark.asyncio
async def test_e2e_contract_live_server() -> None:
    base_url = os.environ["AYB_TEST_URL"]
    collection = os.environ["AYB_TEST_COLLECTION"]
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
