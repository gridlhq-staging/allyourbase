package dev.allyourbase

import java.nio.file.Files
import java.nio.file.Path
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject

object ContractFixtures {
    private val json = Json { ignoreUnknownKeys = true }
    private val fixtureRoot = Path.of("..", "tests", "contract", "fixtures")

    private fun loadJsonObject(relativePath: String): JsonObject {
        val payload = Files.readString(fixtureRoot.resolve(relativePath))
        return json.parseToJsonElement(payload).jsonObject
    }

    private fun parityResponse(name: String): JsonObject =
        loadJsonObject("sdk_parity/$name")["response"]!!.jsonObject

    val magicLinkRequestResponse: JsonObject = loadJsonObject("sdk_contract/magic_link_request_response.json")
    val magicLinkConfirmSuccessResponse: JsonObject = loadJsonObject("sdk_contract/magic_link_confirm_success_response.json")
    val magicLinkConfirmPendingMfaResponse: JsonObject = loadJsonObject("sdk_contract/magic_link_confirm_pending_mfa_response.json")
    val anonymousResponse: JsonObject = parityResponse("anonymous.json")
    val linkEmailResponse: JsonObject = parityResponse("link_email.json")
}
