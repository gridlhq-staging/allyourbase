package dev.allyourbase

import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.jupiter.api.Assumptions.assumeTrue
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class RecordsIntegrationTest {
    @Test
    fun `highlighted search returns snippets on configured server`() = runTest {
        val client = RecordsIntegrationEnv.newClient()
        assumeTrue(client != null)
        val configuredClient = client ?: return@runTest

        val response = configuredClient.records.list(
            collection = RecordsIntegrationEnv.collection,
            params = ListParams(search = "allyourbase", highlight = true),
        )

        val highlights = response.items.mapNotNull { it["_highlight"]?.jsonPrimitive?.contentOrNull }
        assertTrue(highlights.isNotEmpty())
        assertTrue(highlights.any { it.contains("<b>allyourbase</b>") })
    }

    @Test
    fun `fuzzy search matches typo on configured server`() = runTest {
        val client = RecordsIntegrationEnv.newClient()
        assumeTrue(client != null)
        val configuredClient = client ?: return@runTest

        val response = configuredClient.records.list(
            collection = RecordsIntegrationEnv.collection,
            params = ListParams(
                search = "alyourbase",
                fuzzy = true,
                typoThreshold = 0.2,
                facets = listOf("category"),
            ),
        )

        val ids = response.items.mapNotNull { it["id"]?.jsonPrimitive?.contentOrNull }.toSet()
        assertTrue(ids.contains("one"))
        assertTrue(ids.contains("two"))
        val categoryFacets = response.facets?.get("category")?.jsonArray
        assertEquals(1, categoryFacets?.size)
        assertEquals("docs", categoryFacets?.first()?.jsonObject?.get("value")?.jsonPrimitive?.content)
        assertEquals(2, categoryFacets?.first()?.jsonObject?.get("count")?.jsonPrimitive?.content?.toInt())
    }

    @Test
    fun `live env skips when base url is unset`() {
        val client = RecordsIntegrationEnv.newClient(env = emptyMap())

        assertNull(client)
    }

    @Test
    fun `live env does not require an admin token`() {
        val client = RecordsIntegrationEnv.newClient(
            env = mapOf("AYB_TEST_URL" to " http://127.0.0.1:8096/ "),
        )

        assertEquals("http://127.0.0.1:8096", client?.configuration?.baseURL)
        assertNull(client?.token)
    }

    @Test
    fun `live env uses optional admin token when present`() {
        val client = RecordsIntegrationEnv.newClient(
            env = mapOf(
                "AYB_TEST_URL" to "http://127.0.0.1:8096",
                "AYB_TEST_ADMIN_TOKEN" to " admin-token ",
            ),
        )

        assertEquals("admin-token", client?.token)
    }

    @Test
    fun `live env targets canonical harness collection`() {
        assertEquals("sdk_kotlin_search_posts", RecordsIntegrationEnv.collection)
    }
}

private object RecordsIntegrationEnv {
    const val collection = "sdk_kotlin_search_posts"

    fun newClient(env: Map<String, String> = System.getenv()): AYBClient? {
        val baseUrl = env["AYB_TEST_URL"]?.trim()?.trimEnd('/')?.ifEmpty { null }
            ?: return null
        val adminToken = env["AYB_TEST_ADMIN_TOKEN"]?.trim()?.takeIf { it.isNotEmpty() }

        return AYBClient(baseUrl).also { client ->
            adminToken?.let { client.setApiKey(it) }
        }
    }
}
