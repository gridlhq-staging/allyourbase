"""
Stub summary for /Users/stuart/parallel_development/allyourbase_dev/jun04_pm_3_sdk_parity_go_python_openapi/allyourbase_dev/sdk_python/src/allyourbase/records.py.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any, Dict, List, Optional
from urllib.parse import quote, urlencode

from allyourbase.types import BatchOperation, BatchResult, ListResponse

if TYPE_CHECKING:
    from allyourbase.client import AYBClient


class RecordsClient:
    """Handles CRUD operations on collections."""

    def __init__(self, client: AYBClient) -> None:
        self._client = client

    @staticmethod
    def _build_path(path: str, params: Dict[str, str]) -> str:
        if not params:
            return path
        return f"{path}?{urlencode(params)}"

    @staticmethod
    def _collection_path(collection: str) -> str:
        return f"/api/collections/{quote(collection, safe='')}"

    @classmethod
    def _record_path(cls, collection: str, id: str) -> str:
        return f"{cls._collection_path(collection)}/{quote(id, safe='')}"

    async def list(
        self,
        collection: str,
        *,
        page: Optional[int] = None,
        per_page: Optional[int] = None,
        sort: Optional[str] = None,
        filter: Optional[str] = None,
        search: Optional[str] = None,
        fields: Optional[str] = None,
        expand: Optional[str] = None,
        skip_total: bool = False,
        fuzzy: bool = False,
        typo_threshold: Optional[float] = None,
        highlight: bool = False,
        facets: Optional[List[str]] = None,
        semantic: bool = False,
        semantic_query: Optional[str] = None,
    ) -> ListResponse[Dict[str, Any]]:
        params: Dict[str, str] = {}
        if page is not None:
            params["page"] = str(page)
        if per_page is not None:
            params["perPage"] = str(per_page)
        if sort is not None:
            params["sort"] = sort
        if filter is not None:
            params["filter"] = filter
        if search is not None:
            params["search"] = search
        if fields is not None:
            params["fields"] = fields
        if expand is not None:
            params["expand"] = expand
        if skip_total:
            params["skipTotal"] = "true"
        if fuzzy:
            params["fuzzy"] = "true"
        if typo_threshold is not None:
            params["typo_threshold"] = str(typo_threshold)
        if highlight:
            params["highlight"] = "true"
        if facets:
            params["facets"] = ",".join(facets)
        if semantic:
            params["semantic"] = "true"
        if semantic_query:
            params["semantic_query"] = semantic_query

        path = self._build_path(self._collection_path(collection), params)
        resp = await self._client._request(path)
        if resp is None:
            raise RuntimeError("Expected response body for list")
        return ListResponse[Dict[str, Any]].model_validate(resp.json())

    async def get(
        self,
        collection: str,
        id: str,
        *,
        fields: Optional[str] = None,
        expand: Optional[str] = None,
    ) -> Dict[str, Any]:
        params: Dict[str, str] = {}
        if fields is not None:
            params["fields"] = fields
        if expand is not None:
            params["expand"] = expand

        path = self._build_path(self._record_path(collection, id), params)
        resp = await self._client._request(path)
        if resp is None:
            raise RuntimeError("Expected response body for get")
        result: Dict[str, Any] = resp.json()
        return result

    async def create(
        self,
        collection: str,
        data: Dict[str, Any],
    ) -> Dict[str, Any]:
        resp = await self._client._request(
            self._collection_path(collection),
            method="POST",
            json=data,
        )
        if resp is None:
            raise RuntimeError("Expected response body for create")
        result: Dict[str, Any] = resp.json()
        return result

    async def update(
        self,
        collection: str,
        id: str,
        data: Dict[str, Any],
    ) -> Dict[str, Any]:
        resp = await self._client._request(
            self._record_path(collection, id),
            method="PATCH",
            json=data,
        )
        if resp is None:
            raise RuntimeError("Expected response body for update")
        result: Dict[str, Any] = resp.json()
        return result

    async def delete(self, collection: str, id: str) -> None:
        await self._client._request(
            self._record_path(collection, id),
            method="DELETE",
        )

    async def batch(
        self,
        collection: str,
        operations: List[BatchOperation],
    ) -> List[BatchResult[Dict[str, Any]]]:
        resp = await self._client._request(
            f"{self._collection_path(collection)}/batch",
            method="POST",
            json={
                "operations": [op.model_dump(exclude_none=True) for op in operations]
            },
        )
        if resp is None:
            raise RuntimeError("Expected response body for batch")
        raw_list: List[Dict[str, Any]] = resp.json()
        return [BatchResult[Dict[str, Any]].model_validate(item) for item in raw_list]
