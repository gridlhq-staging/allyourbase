package dev.allyourbase

import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable

class RpcLiveIntegrationTest {
    @Test
    @EnabledIfEnvironmentVariable(named = "AYB_REQUIRE_RPC_LIVE", matches = "true")
    fun `sdk contract add returns canonical fixture from live server`() = runTest {
        val client = RpcLiveIntegrationEnv.requiredClient()

        val response = client.rpc("sdk_contract_add", ContractFixtures.rpcRequest)

        assertEquals(ContractFixtures.rpcResponse, response)
    }
}

private object RpcLiveIntegrationEnv {
    fun requiredClient(env: Map<String, String> = System.getenv()): AYBClient {
        val baseUrl = requiredEnv(env, "AYB_TEST_URL").trimEnd('/')
        val adminToken = requiredEnv(env, "AYB_TEST_ADMIN_TOKEN")

        return AYBClient(baseUrl).also { client ->
            client.setApiKey(adminToken)
        }
    }

    private fun requiredEnv(env: Map<String, String>, name: String): String {
        val value = env[name]?.trim()
        require(!value.isNullOrEmpty()) { "Set $name before running live Kotlin RPC tests." }
        return value
    }
}
