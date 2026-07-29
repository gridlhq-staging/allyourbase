package dev.allyourbase

import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class RpcClientTest {
    @Test
    fun `rpc posts fixture body through request seam and returns parsed fixture`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = ContractFixtures.rpcResponse))
        val client = AYBClient("https://api.example.com", transport = transport)
        client.setApiKey("rpc-token")

        val response = client.rpc("sdk_contract_add", ContractFixtures.rpcRequest)

        assertEquals(ContractFixtures.rpcResponse, response)
        val request = transport.requests.single()
        assertEquals("https://api.example.com/api/rpc/sdk_contract_add", request.url)
        assertEquals(HttpMethod.POST, request.method)
        assertEquals("Bearer rpc-token", lowercasedLookup(request.headers, "Authorization"))
        assertEquals("application/json", lowercasedLookup(request.headers, "Accept"))
        assertEquals("application/json", lowercasedLookup(request.headers, "Content-Type"))
        assertArrayEquals(jsonToBytes(ContractFixtures.rpcRequest), request.body)
    }

    @Test
    fun `rpc encodes function name as one RFC 3986 path segment`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 204, body = ByteArray(0)))
        val client = AYBClient("https://api.example.com", transport = transport)

        client.rpc("schema/fn name")

        assertEquals("https://api.example.com/api/rpc/schema%2Ffn%20name", transport.requests.single().url)
    }

    @Test
    fun `rpc returns null for empty success responses`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 204, body = ByteArray(0)))
        transport.enqueue(StubResponse(status = 200, body = ByteArray(0)))
        val client = AYBClient("https://api.example.com", transport = transport)

        assertNull(client.rpc("empty_204"))
        assertNull(client.rpc("empty_200"))
    }

    @Test
    fun `rpc non 2xx propagates normalized AYBException`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(
            StubResponse(
                status = 422,
                json = buildJsonObject {
                    put("message", "invalid rpc arguments")
                    put("code", "rpc/invalid-arguments")
                },
            ),
        )
        val client = AYBClient("https://api.example.com", transport = transport)

        runCatching {
            client.rpc("sdk_contract_add", ContractFixtures.rpcRequest)
        }.onSuccess {
            throw AssertionError("expected AYBException")
        }.onFailure { error ->
            val ayb = error as AYBException
            assertEquals(422, ayb.status)
            assertEquals("invalid rpc arguments", ayb.message)
            assertEquals("rpc/invalid-arguments", ayb.code)
        }
    }
}
