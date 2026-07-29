"""Edge function invocation client.

Thin method-grouping wrapper over ``AYBClient._request``; it adds no independent
transport, auth, or error handling of its own.
"""

from __future__ import annotations

import json as _json
from typing import TYPE_CHECKING, Any, Dict, Optional, Union
from urllib.parse import quote

from allyourbase.types import EdgeInvokeResponse

if TYPE_CHECKING:
    from allyourbase.client import AYBClient


class FunctionsClient:
    """Groups edge-function operations over the shared client seam."""

    def __init__(self, client: "AYBClient") -> None:
        self._client = client

    async def invoke(
        self,
        name: str,
        *,
        body: Optional[Any] = None,
        headers: Optional[Dict[str, str]] = None,
        method: str = "POST",
        skip_auth: bool = False,
    ) -> EdgeInvokeResponse:
        """Invoke an edge function by name and return the raw response envelope.

        Plain objects are serialized to compact JSON and default the
        ``Content-Type`` header; ``str``/``bytes`` bodies are sent verbatim.
        """
        req_headers: Dict[str, str] = dict(headers or {})
        content: Optional[Union[str, bytes]] = None
        if body is not None:
            if isinstance(body, (bytes, bytearray, str)):
                content = bytes(body) if isinstance(body, bytearray) else body
            else:
                content = _json.dumps(body, separators=(",", ":"))
                if not any(key.lower() == "content-type" for key in req_headers):
                    req_headers["Content-Type"] = "application/json"

        resp = await self._client._request(
            f"/functions/v1/{quote(name, safe='')}",
            method=method,
            headers=req_headers or None,
            content=content,
            skip_auth=skip_auth,
            return_response_on_204=True,
        )

        if resp is None:
            raise RuntimeError("AYBClient._request unexpectedly returned None for edge invoke")
        return EdgeInvokeResponse(
            status=resp.status_code,
            headers=dict(resp.headers),
            raw_body=resp.content,
        )
