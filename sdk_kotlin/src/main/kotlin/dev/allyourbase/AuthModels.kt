package dev.allyourbase

import kotlinx.serialization.Serializable
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.SerialName
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonNames

@Serializable
@OptIn(ExperimentalSerializationApi::class)
data class User(
    @JsonNames("userId", "user_id")
    val id: String,
    @JsonNames("emailAddress", "email_address")
    val email: String,
    @JsonNames("is_anonymous")
    val isAnonymous: Boolean? = null,
    @JsonNames("linked_at")
    val linkedAt: String? = null,
    @JsonNames("email_verified")
    val emailVerified: Boolean? = null,
    @JsonNames("created_at", "created")
    val createdAt: String? = null,
    @JsonNames("updated_at", "updated")
    val updatedAt: String? = null,
)

@Serializable
data class AuthResponse(
    val token: String,
    val refreshToken: String,
    val user: User,
)

@Serializable
data class MagicLinkRequestResponse(
    val message: String,
)

@Serializable
data class WebAuthnLoginBeginResponse(
    @SerialName("challenge_id")
    val challengeId: String,
    val options: JsonObject,
)

@Serializable
data class WebAuthnLoginFinishRequest(
    @SerialName("challenge_id")
    val challengeId: String,
    @SerialName("assertion_response")
    val assertionResponse: JsonObject,
)

@Serializable
data class WebAuthnEnrollBeginResponse(
    val attestation: String,
    val authenticatorSelection: JsonObject,
    val challenge: String,
    val pubKeyCredParams: List<JsonObject>,
    val rp: JsonObject,
    val timeout: Int,
    val user: JsonObject,
)

@Serializable
data class WebAuthnEnrollConfirmRequest(
    @SerialName("display_name")
    val displayName: String,
    @SerialName("attestation_response")
    val attestationResponse: JsonObject,
)

@Serializable
data class WebAuthnEnrollConfirmResponse(
    val message: String,
)

@Serializable
data class WebAuthnMfaChallengeResponse(
    @SerialName("challenge_id")
    val challengeId: String,
    val options: JsonObject,
)

@Serializable
data class WebAuthnMfaVerifyRequest(
    @SerialName("challenge_id")
    val challengeId: String,
    @SerialName("assertion_response")
    val assertionResponse: JsonObject,
)

fun interface PasskeyAuthenticator {
    suspend fun createAssertion(options: JsonObject): JsonObject
}

sealed interface MagicLinkConfirmResponse {
    data class Authenticated(val auth: AuthResponse) : MagicLinkConfirmResponse

    data class PendingMfa(val mfaToken: String) : MagicLinkConfirmResponse
}
