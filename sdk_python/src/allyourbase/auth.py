
from __future__ import annotations

from typing import TYPE_CHECKING, Any, Dict
from urllib.parse import quote

from allyourbase.types import (
    AuthResponse,
    MagicLinkConfirmResponse,
    MagicLinkRequestResponse,
    User,
    WebAuthnEnrollBeginResponse,
    WebAuthnEnrollConfirmResponse,
    WebAuthnLoginBeginResponse,
    WebAuthnMFAChallengeResponse,
)

if TYPE_CHECKING:
    from allyourbase.client import AYBClient

_ENCODE_URI_COMPONENT_SAFE = "-_.!~*'()"


class AuthClient:
    """Handles authentication operations."""

    def __init__(self, client: AYBClient) -> None:
        self._client = client

    def _apply_signed_in_session(self, auth: AuthResponse) -> AuthResponse:
        self._client.set_tokens(auth.token, auth.refresh_token)
        self._client._emit_auth_event("SIGNED_IN")
        return auth

    def _webauthn_assertion_body(
        self,
        challenge_id: str,
        assertion_response: Dict[str, Any],
    ) -> Dict[str, Any]:
        return {
            "challenge_id": challenge_id,
            "assertion_response": assertion_response,
        }

    async def register(self, email: str, password: str) -> AuthResponse:
        resp = await self._client._request(
            "/api/auth/register",
            method="POST",
            json={"email": email, "password": password},
        )
        if resp is None:
            raise RuntimeError("Expected response body for register")
        auth = AuthResponse.model_validate(resp.json())
        return self._apply_signed_in_session(auth)

    async def login(self, email: str, password: str) -> AuthResponse:
        resp = await self._client._request(
            "/api/auth/login",
            method="POST",
            json={"email": email, "password": password},
        )
        if resp is None:
            raise RuntimeError("Expected response body for login")
        auth = AuthResponse.model_validate(resp.json())
        return self._apply_signed_in_session(auth)

    async def begin_webauthn_login(self, email: str) -> WebAuthnLoginBeginResponse:
        resp = await self._client._request(
            "/api/auth/webauthn/login/begin",
            method="POST",
            json={"email": email},
        )
        if resp is None:
            raise RuntimeError("Expected response body for begin_webauthn_login")
        return WebAuthnLoginBeginResponse.model_validate(resp.json())

    async def begin_discoverable_login(self) -> WebAuthnLoginBeginResponse:
        resp = await self._client._request(
            "/api/auth/webauthn/login/discover/begin",
            method="POST",
        )
        if resp is None:
            raise RuntimeError("Expected response body for begin_discoverable_login")
        return WebAuthnLoginBeginResponse.model_validate(resp.json())

    async def finish_webauthn_login(
        self,
        challenge_id: str,
        assertion_response: Dict[str, Any],
    ) -> AuthResponse:
        """Finish login with a caller-supplied WebAuthn assertion.

        The SDK does not run browser or platform-authenticator ceremony.
        """
        resp = await self._client._request(
            "/api/auth/webauthn/login/finish",
            method="POST",
            json=self._webauthn_assertion_body(challenge_id, assertion_response),
        )
        if resp is None:
            raise RuntimeError("Expected response body for finish_webauthn_login")
        auth = AuthResponse.model_validate(resp.json())
        return self._apply_signed_in_session(auth)

    async def finish_discoverable_login(
        self,
        challenge_id: str,
        assertion_response: Dict[str, Any],
    ) -> AuthResponse:
        """Finish discoverable login with a caller-supplied WebAuthn assertion.

        The SDK does not run browser or platform-authenticator ceremony.
        """
        resp = await self._client._request(
            "/api/auth/webauthn/login/discover/finish",
            method="POST",
            json=self._webauthn_assertion_body(challenge_id, assertion_response),
        )
        if resp is None:
            raise RuntimeError("Expected response body for finish_discoverable_login")
        auth = AuthResponse.model_validate(resp.json())
        return self._apply_signed_in_session(auth)

    async def enroll_webauthn(self) -> WebAuthnEnrollBeginResponse:
        resp = await self._client._request(
            "/api/auth/mfa/webauthn/enroll",
            method="POST",
        )
        if resp is None:
            raise RuntimeError("Expected response body for enroll_webauthn")
        return WebAuthnEnrollBeginResponse.model_validate(resp.json())

    async def confirm_webauthn_enrollment(
        self,
        display_name: str,
        attestation_response: Dict[str, Any],
    ) -> WebAuthnEnrollConfirmResponse:
        resp = await self._client._request(
            "/api/auth/mfa/webauthn/enroll/confirm",
            method="POST",
            json={
                "display_name": display_name,
                "attestation_response": attestation_response,
            },
        )
        if resp is None:
            raise RuntimeError("Expected response body for confirm_webauthn_enrollment")
        return WebAuthnEnrollConfirmResponse.model_validate(resp.json())

    async def webauthn_challenge(self, mfa_token: str) -> WebAuthnMFAChallengeResponse:
        resp = await self._client._request(
            "/api/auth/mfa/webauthn/challenge",
            method="POST",
            headers={"Authorization": f"Bearer {mfa_token}"},
            skip_auth=True,
        )
        if resp is None:
            raise RuntimeError("Expected response body for webauthn_challenge")
        return WebAuthnMFAChallengeResponse.model_validate(resp.json())

    async def webauthn_verify(
        self,
        mfa_token: str,
        challenge_id: str,
        assertion_response: Dict[str, Any],
    ) -> AuthResponse:
        resp = await self._client._request(
            "/api/auth/mfa/webauthn/verify",
            method="POST",
            headers={"Authorization": f"Bearer {mfa_token}"},
            json=self._webauthn_assertion_body(challenge_id, assertion_response),
            skip_auth=True,
        )
        if resp is None:
            raise RuntimeError("Expected response body for webauthn_verify")
        auth = AuthResponse.model_validate(resp.json())
        return self._apply_signed_in_session(auth)

    async def delete_webauthn(self) -> None:
        await self._client._request(
            "/api/auth/mfa/webauthn/",
            method="DELETE",
        )

    def get_oauth_start_url(
        self,
        provider: str,
        state: str,
        scopes: list[str] | None = None,
        redirect_to: str | None = None,
    ) -> str:
        query = [f"state={quote(state, safe=_ENCODE_URI_COMPONENT_SAFE)}"]
        if scopes:
            query.append(
                f"scopes={quote(','.join(scopes), safe=_ENCODE_URI_COMPONENT_SAFE)}"
            )
        if redirect_to is not None:
            query.append(
                f"redirect_to={quote(redirect_to, safe=_ENCODE_URI_COMPONENT_SAFE)}"
            )
        provider_path = quote(provider, safe="")
        return f"{self._client.base_url}/api/auth/oauth/{provider_path}?{'&'.join(query)}"

    async def sign_in_anonymously(self) -> AuthResponse:
        resp = await self._client._request(
            "/api/auth/anonymous",
            method="POST",
            json={},
        )
        if resp is None:
            raise RuntimeError("Expected response body for sign_in_anonymously")
        auth = AuthResponse.model_validate(resp.json())
        return self._apply_signed_in_session(auth)

    async def request_magic_link(self, email: str) -> MagicLinkRequestResponse:
        resp = await self._client._request(
            "/api/auth/magic-link",
            method="POST",
            json={"email": email},
        )
        if resp is None:
            raise RuntimeError("Expected response body for request_magic_link")
        return MagicLinkRequestResponse.model_validate(resp.json())

    async def confirm_magic_link(self, token: str) -> MagicLinkConfirmResponse:
        resp = await self._client._request(
            "/api/auth/magic-link/confirm",
            method="POST",
            json={"token": token},
        )
        if resp is None:
            raise RuntimeError("Expected response body for confirm_magic_link")
        payload = resp.json()
        if payload.get("mfa_pending") or payload.get("mfaPending"):
            return MagicLinkConfirmResponse.pending(
                mfa_token=payload.get("mfa_token") or payload.get("mfaToken") or ""
            )
        auth = AuthResponse.model_validate(payload)
        self._apply_signed_in_session(auth)
        return MagicLinkConfirmResponse.from_auth(auth)

    async def link_email(self, email: str, password: str) -> AuthResponse:
        resp = await self._client._request(
            "/api/auth/link/email",
            method="POST",
            json={"email": email, "password": password},
        )
        if resp is None:
            raise RuntimeError("Expected response body for link_email")
        auth = AuthResponse.model_validate(resp.json())
        return self._apply_signed_in_session(auth)

    async def me(self) -> User:
        resp = await self._client._request("/api/auth/me")
        if resp is None:
            raise RuntimeError("Expected response body for me")
        return User.model_validate(resp.json())

    async def refresh(self) -> AuthResponse:
        resp = await self._client._request(
            "/api/auth/refresh",
            method="POST",
            json={"refreshToken": self._client.refresh_token},
        )
        if resp is None:
            raise RuntimeError("Expected response body for refresh")
        auth = AuthResponse.model_validate(resp.json())
        self._client.set_tokens(auth.token, auth.refresh_token)
        self._client._emit_auth_event("TOKEN_REFRESHED")
        return auth

    async def logout(self) -> None:
        await self._client._request(
            "/api/auth/logout",
            method="POST",
            json={"refreshToken": self._client.refresh_token},
        )
        self._client.clear_tokens()
        self._client._emit_auth_event("SIGNED_OUT")

    async def delete_account(self) -> None:
        await self._client._request("/api/auth/me", method="DELETE")
        self._client.clear_tokens()
        self._client._emit_auth_event("SIGNED_OUT")

    async def request_password_reset(self, email: str) -> None:
        await self._client._request(
            "/api/auth/password-reset",
            method="POST",
            json={"email": email},
        )

    async def confirm_password_reset(self, token: str, password: str) -> None:
        await self._client._request(
            "/api/auth/password-reset/confirm",
            method="POST",
            json={"token": token, "password": password},
        )

    async def verify_email(self, token: str) -> None:
        await self._client._request(
            "/api/auth/verify",
            method="POST",
            json={"token": token},
        )

    async def resend_verification(self) -> None:
        await self._client._request(
            "/api/auth/verify/resend",
            method="POST",
        )
