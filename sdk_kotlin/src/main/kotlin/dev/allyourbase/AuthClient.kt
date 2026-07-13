package dev.allyourbase

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.encodeToJsonElement
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import kotlinx.serialization.serializer
import java.nio.charset.StandardCharsets

class AuthClient internal constructor(
    private val client: AYBClient,
) {
    private val json = Json { ignoreUnknownKeys = true }

    suspend fun register(email: String, password: String): AuthResponse =
        authenticate(path = "/api/auth/register", email = email, password = password)

    suspend fun login(email: String, password: String): AuthResponse =
        authenticate(path = "/api/auth/login", email = email, password = password)

    suspend fun signInAnonymously(): AuthResponse {
        val response: AuthResponse = client.request(
            path = "/api/auth/anonymous",
            method = HttpMethod.POST,
            body = buildJsonObject {},
            decode = { payload -> decodePayload(payload) },
        )
        return applySignedInSession(response)
    }

    suspend fun requestMagicLink(email: String): MagicLinkRequestResponse =
        client.request(
            path = "/api/auth/magic-link",
            method = HttpMethod.POST,
            body = buildJsonObject { put("email", email) },
            decode = { payload -> decodePayload(payload) },
        )

    suspend fun beginWebAuthnLogin(email: String): WebAuthnLoginBeginResponse =
        client.request(
            path = "/api/auth/webauthn/login/begin",
            method = HttpMethod.POST,
            body = buildJsonObject { put("email", email) },
            decode = { payload -> decodePayload(payload) },
        )

    suspend fun finishWebAuthnLogin(challengeId: String, assertionResponse: JsonObject): AuthResponse {
        val response: AuthResponse = client.request(
            path = "/api/auth/webauthn/login/finish",
            method = HttpMethod.POST,
            body = encodePayload(WebAuthnLoginFinishRequest(challengeId, assertionResponse)),
            decode = { payload -> decodePayload(payload) },
        )
        return applySignedInSession(response)
    }

    suspend fun signInWithPasskey(email: String, authenticator: PasskeyAuthenticator): AuthResponse {
        val begin = beginWebAuthnLogin(email)
        val assertionResponse = authenticator.createAssertion(begin.options)
        return finishWebAuthnLogin(begin.challengeId, assertionResponse)
    }

    /**
     * Build the OAuth provider start URL. The Kotlin SDK only returns the URL; callers
     * are responsible for opening it and handling the redirect or popup flow.
     */
    fun oauthStartUrl(
        provider: String,
        state: String,
        scopes: List<String>? = null,
        redirectTo: String? = null,
    ): String {
        val baseUrl = client.configuration.baseURL.trimEnd('/')
        val queryItems = mutableListOf("state=${state.oauthQueryEncode()}")
        if (!scopes.isNullOrEmpty()) {
            queryItems += "scopes=${scopes.joinToString(",").oauthQueryEncode()}"
        }
        if (!redirectTo.isNullOrEmpty()) {
            queryItems += "redirect_to=${redirectTo.oauthQueryEncode()}"
        }
        // Provider is one path segment: the canonical contract requires "/"
        // and spaces inside it to arrive percent-encoded (the conservative
        // query-safe set is a valid path-segment encoding superset).
        return "$baseUrl/api/auth/oauth/${provider.oauthQueryEncode()}?${queryItems.joinToString("&")}"
    }

    suspend fun enrollWebAuthn(): WebAuthnEnrollBeginResponse =
        client.request(
            path = "/api/auth/mfa/webauthn/enroll",
            method = HttpMethod.POST,
            decode = { payload -> decodePayload(payload) },
        )

    suspend fun confirmWebAuthnEnrollment(
        displayName: String,
        attestationResponse: JsonObject,
    ): WebAuthnEnrollConfirmResponse =
        client.request(
            path = "/api/auth/mfa/webauthn/enroll/confirm",
            method = HttpMethod.POST,
            body = encodePayload(WebAuthnEnrollConfirmRequest(displayName, attestationResponse)),
            decode = { payload -> decodePayload(payload) },
        )

    suspend fun webauthnChallenge(mfaToken: String): WebAuthnMfaChallengeResponse =
        client.request(
            path = "/api/auth/mfa/webauthn/challenge",
            method = HttpMethod.POST,
            headers = mfaAuthorizationHeader(mfaToken),
            skipAuth = true,
            decode = { payload -> decodePayload(payload) },
        )

    suspend fun webauthnVerify(
        mfaToken: String,
        challengeId: String,
        assertionResponse: JsonObject,
    ): AuthResponse {
        val response: AuthResponse = client.request(
            path = "/api/auth/mfa/webauthn/verify",
            method = HttpMethod.POST,
            headers = mfaAuthorizationHeader(mfaToken),
            body = encodePayload(WebAuthnMfaVerifyRequest(challengeId, assertionResponse)),
            skipAuth = true,
            decode = { payload -> decodePayload(payload) },
        )
        return applySignedInSession(response)
    }

    suspend fun deleteWebAuthn() {
        client.request(
            path = "/api/auth/mfa/webauthn",
            method = HttpMethod.DELETE,
            decode = { Unit },
        )
    }

    suspend fun confirmMagicLink(token: String): MagicLinkConfirmResponse {
        val response: MagicLinkConfirmResponse = client.request(
            path = "/api/auth/magic-link/confirm",
            method = HttpMethod.POST,
            body = buildJsonObject { put("token", token) },
            decode = { payload -> decodeMagicLinkConfirmPayload(payload) },
        )
        if (response is MagicLinkConfirmResponse.Authenticated) {
            client.setTokens(response.auth.token, response.auth.refreshToken)
            client.emitAuthState(AuthStateEvent.SIGNED_IN)
        }
        return response
    }

    suspend fun linkEmail(email: String, password: String): AuthResponse =
        authenticate(path = "/api/auth/link/email", email = email, password = password)

    suspend fun me(): User =
        client.request(
            path = "/api/auth/me",
            method = HttpMethod.GET,
            decode = { payload -> decodePayload(payload) },
        )

    suspend fun logout() {
        val refreshToken = requireRefreshToken()
        client.request(
            path = "/api/auth/logout",
            method = HttpMethod.POST,
            body = buildJsonObject { put("refreshToken", refreshToken) },
            decode = { Unit },
        )
        client.clearTokens()
        client.emitAuthState(AuthStateEvent.SIGNED_OUT)
    }

    suspend fun refresh(): AuthResponse {
        val refreshToken = requireRefreshToken()
        val response: AuthResponse = client.request(
            path = "/api/auth/refresh",
            method = HttpMethod.POST,
            body = buildJsonObject { put("refreshToken", refreshToken) },
            decode = { payload -> decodePayload(payload) },
        )
        client.setTokens(response.token, response.refreshToken)
        client.emitAuthState(AuthStateEvent.TOKEN_REFRESHED)
        return response
    }

    private suspend fun authenticate(path: String, email: String, password: String): AuthResponse {
        val response: AuthResponse = client.request(
            path = path,
            method = HttpMethod.POST,
            body = buildJsonObject {
                put("email", email)
                put("password", password)
            },
            decode = { payload -> decodePayload(payload) },
        )
        return applySignedInSession(response)
    }

    private fun requireRefreshToken(): String {
        val refreshToken = client.refreshToken
        if (refreshToken.isNullOrEmpty()) {
            throw AYBException(
                status = 400,
                message = "Missing refresh token",
                code = "auth/missing-refresh-token",
            )
        }
        return refreshToken
    }

    private fun applySignedInSession(response: AuthResponse): AuthResponse {
        client.setTokens(response.token, response.refreshToken)
        client.emitAuthState(AuthStateEvent.SIGNED_IN)
        return response
    }

    private fun mfaAuthorizationHeader(mfaToken: String): Map<String, String> =
        mapOf("Authorization" to "Bearer $mfaToken")

    private inline fun <reified T> encodePayload(payload: T): JsonElement =
        json.encodeToJsonElement(serializer<T>(), payload)

    private inline fun <reified T> decodePayload(payload: JsonElement?): T {
        if (payload == null) {
            throw AYBException(status = 500, message = "Empty response payload")
        }
        return json.decodeFromJsonElement(serializer<T>(), payload)
    }

    private fun decodeMagicLinkConfirmPayload(payload: JsonElement?): MagicLinkConfirmResponse {
        val objectPayload = payload as? JsonObject
            ?: throw AYBException(status = 500, message = "Empty response payload")
        val mfaPending = objectPayload["mfa_pending"]?.jsonPrimitive?.booleanOrNull
            ?: objectPayload["mfaPending"]?.jsonPrimitive?.booleanOrNull
            ?: false
        if (mfaPending) {
            val mfaToken = objectPayload["mfa_token"]?.jsonPrimitive?.contentOrNull
                ?: objectPayload["mfaToken"]?.jsonPrimitive?.contentOrNull
                ?: throw AYBException(status = 500, message = "Missing MFA token")
            return MagicLinkConfirmResponse.PendingMfa(mfaToken)
        }
        return MagicLinkConfirmResponse.Authenticated(json.decodeFromJsonElement(serializer<AuthResponse>(), objectPayload))
    }
}

private fun String.oauthQueryEncode(): String =
    buildString {
        for (byte in this@oauthQueryEncode.toByteArray(StandardCharsets.UTF_8)) {
            val value = byte.toInt() and 0xff
            if (value.isOAuthQuerySafeAscii()) {
                append(value.toChar())
            } else {
                append('%')
                append(value.toString(16).uppercase().padStart(2, '0'))
            }
        }
    }

private fun Int.isOAuthQuerySafeAscii(): Boolean =
    this in 'A'.code..'Z'.code ||
        this in 'a'.code..'z'.code ||
        this in '0'.code..'9'.code ||
        this == '-'.code ||
        this == '_'.code ||
        this == '.'.code ||
        this == '!'.code ||
        this == '~'.code ||
        this == '*'.code ||
        this == '\''.code ||
        this == '('.code ||
        this == ')'.code
