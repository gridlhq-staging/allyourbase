package dev.allyourbase

import kotlinx.serialization.Serializable
import kotlinx.serialization.ExperimentalSerializationApi
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

sealed interface MagicLinkConfirmResponse {
    data class Authenticated(val auth: AuthResponse) : MagicLinkConfirmResponse

    data class PendingMfa(val mfaToken: String) : MagicLinkConfirmResponse
}
