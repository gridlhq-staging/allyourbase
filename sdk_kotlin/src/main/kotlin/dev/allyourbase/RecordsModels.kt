package dev.allyourbase

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put

data class ListParams(
    /** Page number for paginated collection reads. */
    val page: Int? = null,
    /** Page size for paginated collection reads. */
    val perPage: Int? = null,
    /** Sort expression forwarded unchanged to the backend. */
    val sort: String? = null,
    /** Filter expression forwarded unchanged to the backend. */
    val filter: String? = null,
    /** Full-text search query string. */
    val search: String? = null,
    /** Sparse fieldset list forwarded as `fields`. */
    val fields: String? = null,
    /** Expansion list forwarded as `expand`. */
    val expand: String? = null,
    /** Omits total-count work when true. */
    val skipTotal: Boolean? = null,
    /** Enables pg_trgm typo-tolerant search when the backend supports it. */
    val fuzzy: Boolean? = null,
    /** Overrides the backend typo threshold using `typo_threshold`. */
    val typoThreshold: Double? = null,
    /** Requests `_highlight` snippets in each matching item. */
    val highlight: Boolean? = null,
    /** Requests facet counts for the named columns as a comma-joined list. */
    val facets: List<String>? = null,
    /** Opts into semantic search routing when an embedder is configured. */
    val semantic: Boolean? = null,
    /** Supplies the semantic query text serialized as `semantic_query`. */
    val semanticQuery: String? = null,
) {
    fun toQueryItems(): List<Pair<String, String>> {
        val items = mutableListOf<Pair<String, String>>()
        page?.let { items += "page" to it.toString() }
        perPage?.let { items += "perPage" to it.toString() }
        sort?.let { items += "sort" to it }
        filter?.let { items += "filter" to it }
        search?.let { items += "search" to it }
        fields?.let { items += "fields" to it }
        expand?.let { items += "expand" to it }
        if (skipTotal == true) {
            items += "skipTotal" to "true"
        }
        if (fuzzy == true) {
            items += "fuzzy" to "true"
        }
        typoThreshold?.let { items += "typo_threshold" to it.toString() }
        if (highlight == true) {
            items += "highlight" to "true"
        }
        if (!facets.isNullOrEmpty()) {
            items += "facets" to facets.joinToString(",")
        }
        if (semantic == true) {
            items += "semantic" to "true"
        }
        semanticQuery?.let { items += "semantic_query" to it }
        return items
    }
}

data class GetParams(
    val fields: String? = null,
    val expand: String? = null,
) {
    fun toQueryItems(): List<Pair<String, String>> {
        val items = mutableListOf<Pair<String, String>>()
        fields?.let { items += "fields" to it }
        expand?.let { items += "expand" to it }
        return items
    }
}

data class ListMetadata(
    val page: Int,
    val perPage: Int,
    val totalItems: Int,
    val totalPages: Int,
)

data class ListResponse<T>(
    val items: List<T>,
    val metadata: ListMetadata,
    val facets: JsonObject? = null,
) {
    companion object {
        fun <T> decode(
            json: JsonElement?,
            decodeItem: (JsonObject) -> T,
        ): ListResponse<T> {
            val obj = json as? JsonObject
                ?: throw AYBException(status = 500, message = "ListResponse expected object")
            val itemsRaw = obj["items"] as? JsonArray
                ?: throw AYBException(status = 500, message = "ListResponse missing items")

            val items = itemsRaw.map { raw ->
                val item = raw as? JsonObject
                    ?: throw AYBException(status = 500, message = "ListResponse item expected object")
                decodeItem(item)
            }

            return ListResponse(
                items = items,
                metadata = ListMetadata(
                    page = obj.requiredInt("page"),
                    perPage = obj.requiredInt("perPage"),
                    totalItems = obj.requiredInt("totalItems"),
                    totalPages = obj.requiredInt("totalPages"),
                ),
                facets = obj["facets"] as? JsonObject,
            )
        }
    }
}

data class BatchOperation(
    val method: String,
    val id: String? = null,
    val body: JsonObject? = null,
) {
    fun toDictionary(): JsonObject = buildJsonObject {
        put("method", method)
        id?.let { put("id", it) }
        body?.let { put("body", it) }
    }
}

data class BatchResult<T>(
    val index: Int,
    val status: Int,
    val body: T?,
) {
    companion object {
        fun <T> decode(
            json: JsonElement,
            decodeBody: (JsonObject?) -> T?,
        ): BatchResult<T> {
            val obj = json as? JsonObject
                ?: throw AYBException(status = 500, message = "BatchResult expected object")
            val body = (obj["body"] as? JsonObject)
            return BatchResult(
                index = obj.requiredInt("index"),
                status = obj.requiredInt("status"),
                body = decodeBody(body),
            )
        }
    }
}

private fun JsonObject.requiredInt(key: String): Int {
    val primitive = this[key]?.jsonPrimitive ?: throw AYBException(status = 500, message = "missing $key")
    return primitive.intOrNull ?: primitive.content.toIntOrNull()
    ?: throw AYBException(status = 500, message = "invalid int for $key")
}
