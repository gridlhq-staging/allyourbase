package dev.allyourbase

import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable

class EdgeInvokeLiveIntegrationTest {
    @Test
    @EnabledIfEnvironmentVariable(named = "AYB_REQUIRE_EDGE_LIVE", matches = "true")
    fun `sdk contract echo returns canonical raw fixture from live server`() = runTest {
        val client = EdgeInvokeLiveIntegrationEnv.requiredClient()

        val response = client.functions.invoke(
            "sdk-contract-echo",
            EdgeInvokeOptions(body = ContractFixtures.edgeInvokeRequest),
        )

        assertEquals(200, response.status)
        assertArrayEquals(ContractFixtures.edgeInvokeResponseBytes, response.rawBody)
    }
}

private object EdgeInvokeLiveIntegrationEnv {
    fun requiredClient(env: Map<String, String> = System.getenv()): AYBClient {
        val baseUrl = requiredEnv(env, "AYB_TEST_URL").trimEnd('/')
        val adminToken = requiredEnv(env, "AYB_TEST_ADMIN_TOKEN")

        return AYBClient(baseUrl).also { client ->
            client.setApiKey(adminToken)
        }
    }

    private fun requiredEnv(env: Map<String, String>, name: String): String {
        val value = env[name]?.trim()
        require(!value.isNullOrEmpty()) { "Set $name before running live Kotlin edge tests." }
        return value
    }
}
