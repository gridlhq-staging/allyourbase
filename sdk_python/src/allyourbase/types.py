
from __future__ import annotations

from typing import Any, Dict, Generic, List, Literal, Optional, TypeVar

from pydantic import BaseModel, ConfigDict, Field

T = TypeVar("T")


class User(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    id: str
    email: str
    is_anonymous: Optional[bool] = Field(default=None, alias="isAnonymous")
    linked_at: Optional[str] = Field(default=None, alias="linkedAt")
    email_verified: Optional[bool] = Field(default=None, alias="emailVerified")
    created_at: Optional[str] = Field(default=None, alias="createdAt")
    updated_at: Optional[str] = Field(default=None, alias="updatedAt")


class AuthResponse(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    token: str
    refresh_token: str = Field(alias="refreshToken")
    user: User


class MagicLinkRequestResponse(BaseModel):
    message: str


class MagicLinkConfirmResponse(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    token: Optional[str] = None
    refresh_token: Optional[str] = Field(default=None, alias="refreshToken")
    user: Optional[User] = None
    mfa_pending: Optional[bool] = Field(default=None, alias="mfaPending")
    mfa_token: Optional[str] = Field(default=None, alias="mfaToken")

    @property
    def is_pending_mfa(self) -> bool:
        return bool(self.mfa_pending and self.mfa_token)

    @classmethod
    def from_auth(cls, auth: AuthResponse) -> "MagicLinkConfirmResponse":
        return cls(
            token=auth.token,
            refreshToken=auth.refresh_token,
            user=auth.user,
        )

    @classmethod
    def pending(cls, mfa_token: str) -> "MagicLinkConfirmResponse":
        return cls(mfaPending=True, mfaToken=mfa_token)


class ListResponse(BaseModel, Generic[T]):
    model_config = ConfigDict(populate_by_name=True)

    items: List[T]
    page: int
    per_page: int = Field(alias="perPage")
    total_items: int = Field(alias="totalItems")
    total_pages: int = Field(alias="totalPages")


class RealtimeEvent(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    action: str
    table: str
    record: Dict[str, Any]
    old_record: Optional[Dict[str, Any]] = Field(default=None, alias="oldRecord")


class StorageObject(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    id: str
    bucket: str
    name: str
    size: int
    content_type: str = Field(alias="contentType")
    user_id: Optional[str] = Field(default=None, alias="userId")
    created_at: str = Field(alias="createdAt")
    updated_at: Optional[str] = Field(default=None, alias="updatedAt")


class StorageListResponse(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    items: List[StorageObject]
    total_items: int = Field(alias="totalItems")


class BatchOperation(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    method: Literal["create", "update", "delete"]
    id: Optional[str] = None
    body: Optional[Dict[str, Any]] = None


class BatchResult(BaseModel, Generic[T]):
    model_config = ConfigDict(populate_by_name=True)

    index: int
    status: int
    body: Optional[T] = None
