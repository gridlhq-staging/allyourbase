package dev.allyourbase

import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray

class QuerySerializationTest {
    @Test
    fun `list params query item ordering is deterministic`() {
        val items = ListParams(
            page = 2,
            perPage = 50,
            sort = "-created",
            filter = "status='pub'",
            search = "hello",
            fields = "id,title",
            expand = "author",
            skipTotal = true,
            fuzzy = true,
            typoThreshold = 0.2,
            highlight = true,
            facets = listOf("category", "status"),
            semantic = true,
            semanticQuery = "related notes",
        ).toQueryItems()

        assertEquals(
            listOf(
                "page" to "2",
                "perPage" to "50",
                "sort" to "-created",
                "filter" to "status='pub'",
                "search" to "hello",
                "fields" to "id,title",
                "expand" to "author",
                "skipTotal" to "true",
                "fuzzy" to "true",
                "typo_threshold" to "0.2",
                "highlight" to "true",
                "facets" to "category,status",
                "semantic" to "true",
                "semantic_query" to "related notes",
            ),
            items,
        )
    }

    @Test
    fun `get params query items are deterministic`() {
        val items = GetParams(fields = "id,title", expand = "author").toQueryItems()
        assertEquals(listOf("fields" to "id,title", "expand" to "author"), items)
    }

    @Test
    fun `skipTotal only present when true`() {
        assertTrue(ListParams(skipTotal = true).toQueryItems().any { it.first == "skipTotal" })
        assertFalse(ListParams(skipTotal = false).toQueryItems().any { it.first == "skipTotal" })
        assertFalse(ListParams(skipTotal = null).toQueryItems().any { it.first == "skipTotal" })
        assertFalse(ListParams(highlight = false).toQueryItems().any { it.first == "highlight" })
        assertFalse(ListParams(fuzzy = false).toQueryItems().any { it.first == "fuzzy" })
        assertFalse(ListParams(semantic = false).toQueryItems().any { it.first == "semantic" })
        assertFalse(ListParams(facets = emptyList()).toQueryItems().any { it.first == "facets" })
        assertFalse(ListParams(facets = null).toQueryItems().any { it.first == "facets" })
    }

    @Test
    fun `records list full url encoding contains all params`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(
            StubResponse(
                status = 200,
                json = kotlinx.serialization.json.buildJsonObject {
                    putJsonArray("items") {}
                    put("page", 1)
                    put("perPage", 50)
                    put("totalItems", 0)
                    put("totalPages", 0)
                },
            ),
        )

        val client = AYBClient("https://api.example.com", transport = transport)
        client.records.list(
            "posts",
            ListParams(
                page = 1,
                perPage = 50,
                sort = "-created",
                filter = "status='pub'",
                search = "hello world",
                fields = "id,title",
                expand = "author",
                skipTotal = true,
                fuzzy = true,
                typoThreshold = 0.2,
                highlight = true,
                facets = listOf("category", "status"),
                semantic = true,
                semanticQuery = "related notes",
            ),
        )

        val requestUrl = transport.requests.first().url
        assertEquals(
            "https://api.example.com/api/collections/posts?page=1&perPage=50&sort=-created&filter=status%3D%27pub%27&search=hello+world&fields=id%2Ctitle&expand=author&skipTotal=true&fuzzy=true&typo_threshold=0.2&highlight=true&facets=category%2Cstatus&semantic=true&semantic_query=related+notes",
            requestUrl,
        )

        val requestUri = java.net.URI(requestUrl)
        assertEquals("/api/collections/posts", requestUri.path)

        val queryItems = requestUri.query
            .split("&")
            .associate {
                val parts = it.split("=", limit = 2)
                java.net.URLDecoder.decode(parts[0], java.nio.charset.StandardCharsets.UTF_8) to
                    java.net.URLDecoder.decode(parts.getOrNull(1) ?: "", java.nio.charset.StandardCharsets.UTF_8)
            }

        assertEquals("1", queryItems["page"])
        assertEquals("50", queryItems["perPage"])
        assertEquals("-created", queryItems["sort"])
        assertEquals("status='pub'", queryItems["filter"])
        assertEquals("hello world", queryItems["search"])
        assertEquals("id,title", queryItems["fields"])
        assertEquals("author", queryItems["expand"])
        assertEquals("true", queryItems["skipTotal"])
        assertEquals("true", queryItems["fuzzy"])
        assertEquals("0.2", queryItems["typo_threshold"])
        assertEquals("true", queryItems["highlight"])
        assertEquals("category,status", queryItems["facets"])
        assertEquals("true", queryItems["semantic"])
        assertEquals("related notes", queryItems["semantic_query"])
    }
}
