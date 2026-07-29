package dev.allyourbase

import java.nio.file.Files
import java.nio.file.Path
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject

object ContractFixtures {
    private val json = Json { ignoreUnknownKeys = true }
    private val fixtureRoot = Path.of("..", "tests", "contract", "fixtures")

    private fun loadJsonElement(relativePath: String): JsonElement {
        val payload = Files.readString(fixtureRoot.resolve(relativePath))
        return json.parseToJsonElement(payload)
    }

    private fun loadBytes(relativePath: String): ByteArray =
        Files.readAllBytes(fixtureRoot.resolve(relativePath))

    private fun loadResponseBodyBytes(relativePath: String): ByteArray =
        loadBytes(relativePath).let { bytes ->
            if (bytes.lastOrNull() == '\n'.code.toByte()) {
                bytes.copyOf(bytes.size - 1)
            } else {
                bytes
            }
        }

    private fun loadJsonObject(relativePath: String): JsonObject =
        loadJsonElement(relativePath).jsonObject

    private fun loadJsonArray(relativePath: String): JsonArray =
        loadJsonElement(relativePath).jsonArray

    private fun parityResponse(name: String): JsonObject =
        loadJsonObject("sdk_parity/$name")["response"]!!.jsonObject

    val oauthStartUrlCases: JsonArray = loadJsonArray("sdk_contract/oauth_start_url_cases.json")
    val magicLinkRequestResponse: JsonObject = loadJsonObject("sdk_contract/magic_link_request_response.json")
    val magicLinkConfirmSuccessResponse: JsonObject = loadJsonObject("sdk_contract/magic_link_confirm_success_response.json")
    val magicLinkConfirmPendingMfaResponse: JsonObject = loadJsonObject("sdk_contract/magic_link_confirm_pending_mfa_response.json")
    val webAuthnEnrollBeginResponse: JsonObject = loadJsonObject("sdk_contract/webauthn_enroll_begin_response.json")
    val webAuthnEnrollConfirmRequest: JsonObject = loadJsonObject("sdk_contract/webauthn_enroll_confirm_request.json")
    val webAuthnEnrollConfirmResponse: JsonObject = loadJsonObject("sdk_contract/webauthn_enroll_confirm_response.json")
    val webAuthnLoginBeginResponse: JsonObject = loadJsonObject("sdk_contract/webauthn_login_begin_response.json")
    val webAuthnDiscoverBeginResponse: JsonObject = loadJsonObject("sdk_contract/webauthn_discover_begin_response.json")
    val webAuthnDiscoverFinishRequest: JsonObject = loadJsonObject("sdk_contract/webauthn_discover_finish_request.json")
    val webAuthnMfaChallengeResponse: JsonObject = loadJsonObject("sdk_contract/webauthn_mfa_challenge_response.json")
    val webAuthnMfaVerifyRequest: JsonObject = loadJsonObject("sdk_contract/webauthn_mfa_verify_request.json")
    val webAuthnMfaVerifyResponse: JsonObject = loadJsonObject("sdk_contract/webauthn_mfa_verify_response.json")
    val authResponse: JsonObject = loadJsonObject("sdk_contract/auth_response.json")
    val rpcRequest: JsonObject = loadJsonObject("sdk_contract/rpc_request.json")
    val rpcResponse: JsonObject = loadJsonObject("sdk_contract/rpc_response.json")
    val edgeInvokeRequest: JsonObject = loadJsonObject("sdk_contract/edge_invoke_request.json")
    val edgeInvokeResponse: JsonObject = loadJsonObject("sdk_contract/edge_invoke_response.json")
    val edgeInvokeResponseBytes: ByteArray = loadResponseBodyBytes("sdk_contract/edge_invoke_response.json")
    val searchSynonymsRequest: JsonObject = loadJsonObject("sdk_contract/search_synonyms_request.json")
    val searchSynonymsResponse: JsonObject = loadJsonObject("sdk_contract/search_synonyms_response.json")
    val anonymousResponse: JsonObject = parityResponse("anonymous.json")
    val linkEmailResponse: JsonObject = parityResponse("link_email.json")
}
