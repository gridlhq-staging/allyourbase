package dev.allyourbase

class FunctionsClient internal constructor(
    private val client: AYBClient,
) {
    suspend fun invoke(
        name: String,
        options: EdgeInvokeOptions = EdgeInvokeOptions(),
    ): EdgeInvokeResponse {
        val response = client.requestRaw(
            path = "/functions/v1/${encodePathSegment(name)}",
            method = options.method,
            headers = options.headers,
            body = options.body,
            rawBody = options.rawBody,
            skipAuth = options.skipAuth,
        )
        return EdgeInvokeResponse(
            status = response.statusCode,
            headers = response.headers,
            rawBody = response.body,
        )
    }
}
