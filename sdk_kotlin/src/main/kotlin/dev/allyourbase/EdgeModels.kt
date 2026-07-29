package dev.allyourbase

import kotlinx.serialization.json.JsonElement

data class EdgeInvokeOptions(
    val method: String = HttpMethod.POST.name,
    val headers: Map<String, String> = emptyMap(),
    val body: JsonElement? = null,
    val rawBody: ByteArray? = null,
    val skipAuth: Boolean = false,
) {
    init {
        require(body == null || rawBody == null) {
            "body and rawBody are mutually exclusive"
        }
    }
}

data class EdgeInvokeResponse(
    val status: Int,
    val headers: Map<String, String>,
    val rawBody: ByteArray,
)
