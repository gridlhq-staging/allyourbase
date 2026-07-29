"""AllYourBase Python SDK."""

from __future__ import annotations

from allyourbase.errors import AYBError
from allyourbase.types import (
    AuthResponse,
    BatchOperation,
    BatchResult,
    EdgeInvokeResponse,
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
    WebAuthnEnrollBeginResponse,
    WebAuthnEnrollConfirmRequest,
    WebAuthnEnrollConfirmResponse,
    WebAuthnLoginBeginResponse,
    WebAuthnMFAChallengeResponse,
    WebAuthnMFAVerifyRequest,
)

__all__ = [
    "AYBClient",
    "AYBError",
    "AuthResponse",
    "BatchOperation",
    "BatchResult",
    "EdgeInvokeResponse",
    "ListResponse",
    "MagicLinkConfirmResponse",
    "MagicLinkRequestResponse",
    "RealtimeEvent",
    "SearchSynonymGroup",
    "SearchSynonymsRequest",
    "SearchSynonymsResponse",
    "StorageListResponse",
    "StorageObject",
    "User",
    "WebAuthnEnrollBeginResponse",
    "WebAuthnEnrollConfirmRequest",
    "WebAuthnEnrollConfirmResponse",
    "WebAuthnLoginBeginResponse",
    "WebAuthnMFAChallengeResponse",
    "WebAuthnMFAVerifyRequest",
]


def __getattr__(name: str) -> object:
    if name == "AYBClient":
        from allyourbase.client import AYBClient

        return AYBClient
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")
