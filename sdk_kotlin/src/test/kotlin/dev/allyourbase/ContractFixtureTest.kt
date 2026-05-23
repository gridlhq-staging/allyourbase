package dev.allyourbase

import kotlinx.serialization.json.Json
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.jsonPrimitive
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Test

class ContractFixtureTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test
    fun `auth response and user fixtures decode`() {
        val response = json.decodeFromJsonElement(AuthResponse.serializer(), ContractFixtures.linkEmailResponse)
        assertNotNull(response.token)
        assertNotNull(response.refreshToken)
        assertEquals("upgraded@example.com", response.user.email)
        assertNull(response.user.emailVerified)
        assertNotNull(response.user.linkedAt)
        assertNotNull(response.user.createdAt)
        assertNotNull(response.user.updatedAt)

        val anonymous = json.decodeFromJsonElement(AuthResponse.serializer(), ContractFixtures.anonymousResponse)
        assertEquals(true, anonymous.user.isAnonymous)
    }

    @Test
    fun `magic link fixtures decode with canonical aliases`() = runTest {
        val request = json.decodeFromJsonElement(MagicLinkRequestResponse.serializer(), ContractFixtures.magicLinkRequestResponse)
        assertEquals("If an account exists, a magic link has been sent.", request.message)

        val successTransport = MockHttpTransport()
        successTransport.enqueue(StubResponse(status = 200, json = ContractFixtures.magicLinkConfirmSuccessResponse))
        val successClient = AYBClient("https://api.example.com", transport = successTransport)
        val success = successClient.auth.confirmMagicLink("fixture-token-success")
        when (success) {
            is MagicLinkConfirmResponse.Authenticated -> {
                assertEquals("jwt_magic_link", success.auth.token)
                assertEquals("refresh_magic_link", success.auth.refreshToken)
                assertEquals("magic@allyourbase.io", success.auth.user.email)
                assertEquals(true, success.auth.user.emailVerified)
                assertEquals("2026-05-01T12:00:00Z", success.auth.user.createdAt)
                assertNull(success.auth.user.updatedAt)
            }
            is MagicLinkConfirmResponse.PendingMfa -> throw AssertionError("expected authenticated fixture response")
        }

        val pendingTransport = MockHttpTransport()
        pendingTransport.enqueue(StubResponse(status = 200, json = ContractFixtures.magicLinkConfirmPendingMfaResponse))
        val pendingClient = AYBClient("https://api.example.com", transport = pendingTransport)
        val pending = pendingClient.auth.confirmMagicLink("fixture-token-pending")
        when (pending) {
            is MagicLinkConfirmResponse.PendingMfa -> assertEquals("mfa_pending_token_stage1", pending.mfaToken)
            is MagicLinkConfirmResponse.Authenticated -> throw AssertionError("expected pending mfa fixture response")
        }
    }

    @Test
    fun `list response fixture decodes metadata and items`() {
        val payload = """
            {
              "items": [
                {"id": "rec_1", "title": "First"},
                {"id": "rec_2", "title": "Second"}
              ],
              "page": 1,
              "perPage": 2,
              "totalItems": 2,
              "totalPages": 1
            }
        """.trimIndent()

        val decoded = ListResponse.decode(json.parseToJsonElement(payload)) { it }
        assertEquals(2, decoded.items.size)
        assertEquals(2, decoded.metadata.totalItems)
        assertEquals("rec_1", decoded.items.first()["id"]?.toString()?.trim('"'))
    }

    @Test
    fun `error fixture with numeric code maps to string`() {
        val response = HttpResponse(
            statusCode = 403,
            statusText = "Forbidden",
            headers = emptyMap(),
            body = """
                {
                  "code": 403,
                  "message": "forbidden",
                  "data": {"resource": "posts"},
                  "doc_url": "https://allyourbase.io/docs/errors#forbidden"
                }
            """.trimIndent().encodeToByteArray(),
        )

        val error = AYBException.from(response)
        assertEquals("403", error.code)
        assertEquals("forbidden", error.message)
        assertEquals("posts", error.data?.get("resource")?.toString()?.trim('"'))
    }

    @Test
    fun `error fixture with string code stays string`() {
        val response = HttpResponse(
            statusCode = 400,
            statusText = "Bad Request",
            headers = emptyMap(),
            body = """
                {
                  "code": "auth/missing-refresh-token",
                  "message": "Missing refresh token",
                  "data": {"detail": "refresh token not available"}
                }
            """.trimIndent().encodeToByteArray(),
        )

        val error = AYBException.from(response)
        assertEquals("auth/missing-refresh-token", error.code)
        assertEquals("Missing refresh token", error.message)
    }

    @Test
    fun `storage object fixture decodes snake and camel content type`() {
        val snake = """
            {
              "id": "file_abc123",
              "bucket": "uploads",
              "name": "document.pdf",
              "size": 1024,
              "content_type": "application/pdf",
              "user_id": "usr_1",
              "created_at": "2026-01-01T00:00:00Z",
              "updated_at": "2026-01-02T12:30:00Z"
            }
        """.trimIndent()

        val camel = """
            {
              "id": "file_abc123",
              "bucket": "uploads",
              "name": "document.pdf",
              "size": 1024,
              "contentType": "application/pdf",
              "userId": "usr_1",
              "createdAt": "2026-01-01T00:00:00Z",
              "updatedAt": "2026-01-02T12:30:00Z"
            }
        """.trimIndent()

        val one = json.decodeFromString<StorageObject>(snake)
        val two = json.decodeFromString<StorageObject>(camel)

        assertEquals("application/pdf", one.contentType)
        assertEquals("usr_1", one.userId)
        assertEquals("application/pdf", two.contentType)
        assertEquals("usr_1", two.userId)
    }

    @Test
    fun `storage list fixture decodes envelope and aliases`() {
        val snake = """
            {
              "items": [
                {
                  "id": "file_1",
                  "bucket": "uploads",
                  "name": "doc1.pdf",
                  "size": 1024,
                  "content_type": "application/pdf",
                  "user_id": "usr_1",
                  "created_at": "2026-01-01T00:00:00Z",
                  "updated_at": null
                },
                {
                  "id": "file_2",
                  "bucket": "uploads",
                  "name": "image.png",
                  "size": 2048,
                  "content_type": "image/png",
                  "user_id": null,
                  "created_at": "2026-01-02T00:00:00Z",
                  "updated_at": null
                }
              ],
              "total_items": 2
            }
        """.trimIndent()

        val camel = """
            {
              "items": [
                {
                  "id": "file_1",
                  "bucket": "uploads",
                  "name": "doc1.pdf",
                  "size": 1024,
                  "contentType": "application/pdf",
                  "userId": "usr_1",
                  "createdAt": "2026-01-01T00:00:00Z",
                  "updatedAt": null
                },
                {
                  "id": "file_2",
                  "bucket": "uploads",
                  "name": "image.png",
                  "size": 2048,
                  "contentType": "image/png",
                  "userId": null,
                  "createdAt": "2026-01-02T00:00:00Z",
                  "updatedAt": null
                }
              ],
              "totalItems": 2
            }
        """.trimIndent()

        val snakeDecoded = json.decodeFromString<StorageListResponse>(snake)
        val camelDecoded = json.decodeFromString<StorageListResponse>(camel)

        assertEquals(2, snakeDecoded.totalItems)
        assertEquals("application/pdf", snakeDecoded.items.first().contentType)
        assertEquals("usr_1", snakeDecoded.items.first().userId)
        assertEquals("image/png", snakeDecoded.items[1].contentType)
        assertNull(snakeDecoded.items[1].userId)

        assertEquals(2, camelDecoded.totalItems)
        assertEquals("application/pdf", camelDecoded.items.first().contentType)
        assertEquals("usr_1", camelDecoded.items.first().userId)
        assertEquals("image/png", camelDecoded.items[1].contentType)
        assertNull(camelDecoded.items[1].userId)
    }

    @Test
    fun `realtime event fixture decodes snake and camel old record aliases`() {
        val snake = """
            {
              "action": "UPDATE",
              "table": "posts",
              "record": {"id": "rec_1", "title": "after"},
              "old_record": {"id": "rec_1", "title": "before"}
            }
        """.trimIndent()

        val camel = """
            {
              "action": "UPDATE",
              "table": "posts",
              "record": {"id": "rec_1", "title": "after"},
              "oldRecord": {"id": "rec_1", "title": "before"}
            }
        """.trimIndent()

        val snakeDecoded = json.decodeFromString<RealtimeEvent>(snake)
        val camelDecoded = json.decodeFromString<RealtimeEvent>(camel)

        assertEquals("UPDATE", snakeDecoded.action)
        assertEquals("before", snakeDecoded.oldRecord?.get("title")?.jsonPrimitive?.content)
        assertEquals("rec_1", snakeDecoded.record["id"]?.jsonPrimitive?.content)

        assertEquals("UPDATE", camelDecoded.action)
        assertEquals("rec_1", camelDecoded.record["id"]?.jsonPrimitive?.content)
        assertEquals("before", camelDecoded.oldRecord?.get("title")?.jsonPrimitive?.content)
    }
}
