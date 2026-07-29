package dev.allyourbase

import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class EdgeInvokeClientTest {
    @Test
    fun `invoke posts fixture json body with request builder headers`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(
            StubResponse(
                status = 200,
                headers = mapOf("x-edge-run" to "deterministic"),
                body = ContractFixtures.edgeInvokeResponseBytes,
            ),
        )
        val client = AYBClient("https://api.example.com", transport = transport)
        client.setApiKey("edge-token")

        val response = client.functions.invoke(
            "sdk-contract-echo",
            EdgeInvokeOptions(
                body = ContractFixtures.edgeInvokeRequest,
                headers = mapOf("X-Trace-Id" to "trace-1"),
            ),
        )

        val request = transport.requests.single()
        assertEquals("https://api.example.com/functions/v1/sdk-contract-echo", request.url)
        assertEquals(HttpMethod.POST, request.method)
        assertEquals("application/json", lowercasedLookup(request.headers, "Accept"))
        assertEquals("Bearer edge-token", lowercasedLookup(request.headers, "Authorization"))
        assertEquals("trace-1", lowercasedLookup(request.headers, "X-Trace-Id"))
        assertEquals("application/json", lowercasedLookup(request.headers, "Content-Type"))
        assertArrayEquals(jsonToBytes(ContractFixtures.edgeInvokeRequest), request.body)
        assertEquals(200, response.status)
        assertEquals("deterministic", lowercasedLookup(response.headers, "x-edge-run"))
        assertArrayEquals(ContractFixtures.edgeInvokeResponseBytes, response.rawBody)
    }

    @Test
    fun `invoke encodes function name as one RFC 3986 path segment`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 204, body = ByteArray(0)))
        val client = AYBClient("https://api.example.com", transport = transport)

        client.functions.invoke("schema/fn name")

        assertEquals("https://api.example.com/functions/v1/schema%2Ffn%20name", transport.requests.single().url)
    }

    @Test
    fun `custom transport receives arbitrary caller method through public request method`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 204, body = ByteArray(0)))
        val client = AYBClient("https://api.example.com", transport = transport)

        client.functions.invoke(
            "sdk-contract-echo",
            EdgeInvokeOptions(method = "OPTIONS"),
        )

        assertEquals("OPTIONS", transport.requests.single().method.name)
    }

    @Test
    fun `invoke sends verbatim raw request bytes`() = runTest {
        val rawBody = "plain text: not json".encodeToByteArray()
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 204, body = ByteArray(0)))
        val client = AYBClient("https://api.example.com", transport = transport)

        client.functions.invoke(
            "sdk-contract-echo",
            EdgeInvokeOptions(
                rawBody = rawBody,
                headers = mapOf("Content-Type" to "text/plain"),
            ),
        )

        val request = transport.requests.single()
        assertArrayEquals(rawBody, request.body)
        assertEquals("text/plain", lowercasedLookup(request.headers, "Content-Type"))
    }

    @Test
    fun `invoke rejects simultaneous json and raw request bodies`() {
        assertThrows(IllegalArgumentException::class.java) {
            EdgeInvokeOptions(
                body = ContractFixtures.edgeInvokeRequest,
                rawBody = "ambiguous".encodeToByteArray(),
            )
        }
    }

    @Test
    fun `skipAuth omits bearer and preserves normalized errors`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(
            StubResponse(
                status = 403,
                json = buildJsonObject {
                    put("message", "edge denied")
                    put("code", "edge/denied")
                    put("data", buildJsonObject { put("function", "sdk-contract-echo") })
                    put("doc_url", "https://allyourbase.io/docs/errors#edge-denied")
                },
            ),
        )
        val client = AYBClient("https://api.example.com", transport = transport)
        client.setApiKey("edge-token")

        runCatching {
            client.functions.invoke("sdk-contract-echo", EdgeInvokeOptions(skipAuth = true))
        }.onSuccess {
            throw AssertionError("expected AYBException")
        }.onFailure { error ->
            val ayb = error as AYBException
            assertEquals(403, ayb.status)
            assertEquals("edge denied", ayb.message)
            assertEquals("edge/denied", ayb.code)
            assertEquals("sdk-contract-echo", ayb.data?.get("function")?.toString()?.trim('"'))
            assertEquals("https://allyourbase.io/docs/errors#edge-denied", ayb.docUrl)
        }
        assertNull(lowercasedLookup(transport.requests.single().headers, "Authorization"))
    }

    @Test
    fun `invoke preserves 202 status headers and raw bytes`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(
            StubResponse(
                status = 202,
                headers = mapOf("x-edge-status" to "accepted"),
                body = ContractFixtures.edgeInvokeResponseBytes,
            ),
        )
        val client = AYBClient("https://api.example.com", transport = transport)

        val response = client.functions.invoke("sdk-contract-echo")

        assertEquals(202, response.status)
        assertEquals("accepted", lowercasedLookup(response.headers, "x-edge-status"))
        assertArrayEquals(ContractFixtures.edgeInvokeResponseBytes, response.rawBody)
    }

    @Test
    fun `invoke preserves 204 headers with empty raw body`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(
            StubResponse(
                status = 204,
                headers = mapOf("x-edge-empty" to "true"),
                body = ByteArray(0),
            ),
        )
        val client = AYBClient("https://api.example.com", transport = transport)

        val response = client.functions.invoke("sdk-contract-echo")

        assertEquals(204, response.status)
        assertEquals("true", lowercasedLookup(response.headers, "x-edge-empty"))
        assertArrayEquals(ByteArray(0), response.rawBody)
    }
}
