package dev.allyourbase

import io.ktor.client.HttpClient
import io.ktor.client.call.body
import io.ktor.client.engine.okhttp.OkHttp
import io.ktor.client.plugins.HttpTimeout
import io.ktor.client.request.header
import io.ktor.client.request.request
import io.ktor.client.request.setBody
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds

data class HttpMethod(val name: String) {
    companion object {
        val GET = HttpMethod("GET")
        val POST = HttpMethod("POST")
        val PUT = HttpMethod("PUT")
        val PATCH = HttpMethod("PATCH")
        val DELETE = HttpMethod("DELETE")
    }
}

data class HttpRequest(
    val url: String,
    val method: HttpMethod,
    val headers: Map<String, String>,
    val body: ByteArray?,
)

data class HttpResponse(
    val statusCode: Int,
    val statusText: String,
    val headers: Map<String, String>,
    val body: ByteArray,
)

interface HttpTransport {
    suspend fun send(request: HttpRequest): HttpResponse
}

class KtorHttpTransport(
    timeout: Duration = 30.seconds,
    client: HttpClient? = null,
) : HttpTransport {
    private val client: HttpClient = client ?: HttpClient(OkHttp) {
        install(HttpTimeout) {
            requestTimeoutMillis = timeout.inWholeMilliseconds
            connectTimeoutMillis = timeout.inWholeMilliseconds
            socketTimeoutMillis = timeout.inWholeMilliseconds
        }
    }

    override suspend fun send(request: HttpRequest): HttpResponse {
        val response = client.request(request.url) {
            method = io.ktor.http.HttpMethod(request.method.name)
            request.headers.forEach { (name, value) ->
                header(name, value)
            }
            request.body?.let { setBody(it) }
        }

        val headers = response.headers.entries().associate { (name, values) ->
            name to values.joinToString(",")
        }

        return HttpResponse(
            statusCode = response.status.value,
            statusText = response.status.description,
            headers = headers,
            body = response.body(),
        )
    }
}
