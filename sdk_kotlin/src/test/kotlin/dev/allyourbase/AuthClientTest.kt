package dev.allyourbase

import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonObject
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class AuthClientTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test
    fun `register and login send expected request shape`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = authResponseJson("r1", "rr1")))
        transport.enqueue(StubResponse(status = 200, json = authResponseJson("r2", "rr2")))

        val client = AYBClient("https://api.example.com", transport = transport)

        val register = client.auth.register("dev@allyourbase.io", "secret")
        val login = client.auth.login("dev@allyourbase.io", "secret")

        assertEquals("r1", register.token)
        assertEquals("r2", login.token)

        val first = transport.requests[0]
        assertEquals(HttpMethod.POST, first.method)
        assertEquals("/api/auth/register", java.net.URI(first.url).path)
        val firstBody = json.parseToJsonElement(first.body!!.decodeToString()) as JsonObject
        assertEquals("dev@allyourbase.io", firstBody["email"]!!.jsonPrimitive.content)
        assertEquals("secret", firstBody["password"]!!.jsonPrimitive.content)

        val second = transport.requests[1]
        assertEquals(HttpMethod.POST, second.method)
        assertEquals("/api/auth/login", java.net.URI(second.url).path)
    }

    @Test
    fun `me uses bearer token`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(
            StubResponse(
                status = 200,
                json = buildJsonObject {
                    put("id", "usr_1")
                    put("email", "dev@allyourbase.io")
                },
            ),
        )

        val client = AYBClient("https://api.example.com", transport = transport)
        client.setTokens("jwt_1", "refresh_1")

        val me = client.auth.me()

        assertEquals("usr_1", me.id)
        assertEquals("Bearer jwt_1", lowercasedLookup(transport.requests.first().headers, "authorization"))
    }

    @Test
    fun `sign in anonymously stores tokens and emits signed in`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 201, json = ContractFixtures.anonymousResponse))
        val client = AYBClient("https://api.example.com", transport = transport)

        val events = mutableListOf<AuthStateEvent>()
        client.onAuthStateChange { event, _ -> events.add(event) }

        val response = client.auth.signInAnonymously()

        assertTrue(response.user.isAnonymous == true)
        assertEquals(response.token, client.token)
        assertEquals(response.refreshToken, client.refreshToken)
        assertTrue(events.contains(AuthStateEvent.SIGNED_IN))

        val request = transport.requests.single()
        assertEquals(HttpMethod.POST, request.method)
        assertEquals("/api/auth/anonymous", java.net.URI(request.url).path)
        val body = json.parseToJsonElement(request.body!!.decodeToString()) as JsonObject
        assertTrue(body.isEmpty())
    }

    @Test
    fun `request magic link posts email without mutating tokens`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = ContractFixtures.magicLinkRequestResponse))
        val client = AYBClient("https://api.example.com", transport = transport)

        val response = client.auth.requestMagicLink("fixture@example.com")

        assertEquals("If an account exists, a magic link has been sent.", response.message)
        assertNull(client.token)
        assertNull(client.refreshToken)

        val request = transport.requests.single()
        assertEquals("/api/auth/magic-link", java.net.URI(request.url).path)
        val body = json.parseToJsonElement(request.body!!.decodeToString()) as JsonObject
        assertEquals("fixture@example.com", body["email"]!!.jsonPrimitive.content)
    }

    @Test
    fun `begin webauthn login posts email and decodes challenge response`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = ContractFixtures.webAuthnLoginBeginResponse))
        val client = AYBClient("https://api.example.com", transport = transport)

        val response = client.auth.beginWebAuthnLogin("passkey@example.com")

        assertEquals("webauthn_challenge_fixture", response.challengeId)
        assertEquals("webauthn_login_begin_challenge", response.options["challenge"]!!.jsonPrimitive.content)
        assertNull(client.token)
        assertNull(client.refreshToken)

        val request = transport.requests.single()
        assertEquals(HttpMethod.POST, request.method)
        assertEquals("/api/auth/webauthn/login/begin", java.net.URI(request.url).path)
        val body = json.parseToJsonElement(request.body!!.decodeToString()) as JsonObject
        assertEquals("passkey@example.com", body["email"]!!.jsonPrimitive.content)
    }

    @Test
    fun `finish webauthn login serializes snake case assertion request and signs in`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = ContractFixtures.authResponse))
        val client = AYBClient("https://api.example.com", transport = transport)
        val events = mutableListOf<AuthStateEvent>()
        client.onAuthStateChange { event, _ -> events.add(event) }

        val assertionResponse = buildJsonObject {
            put("id", "credential-id")
            put("type", "public-key")
        }
        val response = client.auth.finishWebAuthnLogin("webauthn_challenge_fixture", assertionResponse)

        assertEquals("jwt_stage3", response.token)
        assertEquals("refresh_stage3", client.refreshToken)
        assertEquals("jwt_stage3", client.token)
        assertEquals(listOf(AuthStateEvent.SIGNED_IN), events)

        val request = transport.requests.single()
        assertEquals(HttpMethod.POST, request.method)
        assertEquals("/api/auth/webauthn/login/finish", java.net.URI(request.url).path)
        val body = json.parseToJsonElement(request.body!!.decodeToString()) as JsonObject
        assertEquals("webauthn_challenge_fixture", body["challenge_id"]!!.jsonPrimitive.content)
        assertEquals("credential-id", body["assertion_response"]!!.jsonObject["id"]!!.jsonPrimitive.content)
        assertNull(body["challengeId"])
        assertNull(body["assertionResponse"])
    }

    @Test
    fun `oauth start url matches contract fixture path and query bytes`() {
        assertTrue(ContractFixtures.oauthStartUrlCases.isNotEmpty())

        for (caseElement in ContractFixtures.oauthStartUrlCases) {
            val fixtureCase = caseElement.jsonObject
            val baseUrl = fixtureCase["base_url"]!!.jsonPrimitive.content
            val provider = fixtureCase["provider"]!!.jsonPrimitive.content
            val state = fixtureCase["state"]!!.jsonPrimitive.content
            val scopes = fixtureCase["scopes"]?.jsonArray?.map { it.jsonPrimitive.content }
            val redirectTo = fixtureCase["redirect_to"]?.jsonPrimitive?.contentOrNull
            val expectedPathQuery = fixtureCase["expected_path_query"]!!.jsonPrimitive.content
            val client = AYBClient(baseUrl, transport = MockHttpTransport())

            val actualUri = java.net.URI(client.auth.oauthStartUrl(provider, state, scopes, redirectTo))
            val actualPathQuery = if (actualUri.rawQuery == null) {
                actualUri.rawPath
            } else {
                "${actualUri.rawPath}?${actualUri.rawQuery}"
            }

            assertEquals(expectedPathQuery, actualPathQuery)
            assertEquals(
                expectedPathQuery.substringAfter("?", "").split("&").map { it.substringBefore("=") },
                actualUri.rawQuery!!.split("&").map { it.substringBefore("=") },
            )
            // Parse from rawQuery: a decoded query cannot be split on "&" once
            // state itself contains a literal "&".
            val rawState = actualUri.rawQuery!!.split("&").first { it.startsWith("state=") }.substringAfter("=")
            assertEquals(state, java.net.URLDecoder.decode(rawState, Charsets.UTF_8))
            if (scopes == null) {
                assertFalse(actualUri.rawQuery!!.contains("scopes="))
            } else {
                assertTrue(actualUri.rawQuery!!.contains("scopes="))
            }
            if (redirectTo == null) {
                assertFalse(actualUri.rawQuery!!.contains("redirect_to="))
            } else {
                assertTrue(actualUri.rawQuery!!.contains("redirect_to="))
                if (redirectTo.contains(" ")) {
                    assertTrue(actualUri.rawQuery!!.contains("%20"))
                }
            }
        }
    }

    @Test
    fun `oauth start url query encoding matches javascript encodeURIComponent`() {
        val client = AYBClient("https://api.example.com/", transport = MockHttpTransport())

        val url = client.auth.oauthStartUrl(
            provider = "google",
            state = "raw_state",
            scopes = listOf("openid", "profile email", "custom:scope"),
            redirectTo = "https://app.example.com/~user!x*(y)'?next=a b&mark=\u2713",
        )

        assertEquals(
            "https://api.example.com/api/auth/oauth/google?state=raw_state" +
                "&scopes=openid%2Cprofile%20email%2Ccustom%3Ascope" +
                "&redirect_to=https%3A%2F%2Fapp.example.com%2F~user!x*(y)'%3Fnext%3Da%20b%26mark%3D%E2%9C%93",
            url,
        )
    }

    @Test
    fun `sign in with passkey composes begin authenticator and finish`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = ContractFixtures.webAuthnLoginBeginResponse))
        transport.enqueue(StubResponse(status = 200, json = ContractFixtures.authResponse))
        val client = AYBClient("https://api.example.com", transport = transport)
        val events = mutableListOf<AuthStateEvent>()
        client.onAuthStateChange { event, _ -> events.add(event) }

        val expectedBegin = json.decodeFromJsonElement(
            WebAuthnLoginBeginResponse.serializer(),
            ContractFixtures.webAuthnLoginBeginResponse,
        )
        val assertionResponse = buildPasskeyAssertionResponse()
        val authenticatorRequests = mutableListOf<JsonObject>()
        val authenticator = PasskeyAuthenticator { options ->
            authenticatorRequests.add(options)
            assertionResponse
        }

        val response = client.auth.signInWithPasskey("passkey@example.com", authenticator)

        assertEquals("jwt_stage3", response.token)
        assertEquals("jwt_stage3", client.token)
        assertEquals("refresh_stage3", client.refreshToken)
        assertEquals(listOf(AuthStateEvent.SIGNED_IN), events)
        assertEquals(listOf(expectedBegin.options), authenticatorRequests)

        assertEquals(2, transport.requests.size)
        val beginRequest = transport.requests[0]
        assertEquals(HttpMethod.POST, beginRequest.method)
        assertEquals("/api/auth/webauthn/login/begin", java.net.URI(beginRequest.url).path)
        val beginBody = json.parseToJsonElement(beginRequest.body!!.decodeToString()) as JsonObject
        assertEquals("passkey@example.com", beginBody["email"]!!.jsonPrimitive.content)

        val finishRequest = transport.requests[1]
        assertEquals(HttpMethod.POST, finishRequest.method)
        assertEquals("/api/auth/webauthn/login/finish", java.net.URI(finishRequest.url).path)
        val finishBody = json.parseToJsonElement(finishRequest.body!!.decodeToString()) as JsonObject
        assertEquals("webauthn_challenge_fixture", finishBody["challenge_id"]!!.jsonPrimitive.content)
        assertEquals(assertionResponse, finishBody["assertion_response"])
        assertNull(finishBody["challengeId"])
        assertNull(finishBody["assertionResponse"])
    }

    @Test
    fun `webauthn mfa enrollment helpers use session bearer without signing in again`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = ContractFixtures.webAuthnEnrollBeginResponse))
        transport.enqueue(StubResponse(status = 200, json = ContractFixtures.webAuthnEnrollConfirmResponse))
        transport.enqueue(StubResponse(status = 204, body = ByteArray(0)))
        val client = AYBClient("https://api.example.com", transport = transport)
        client.setTokens("jwt_existing", "refresh_existing")
        val events = mutableListOf<AuthStateEvent>()
        client.onAuthStateChange { event, _ -> events.add(event) }

        val begin = client.auth.enrollWebAuthn()
        val confirm = client.auth.confirmWebAuthnEnrollment(
            "Primary security key",
            ContractFixtures.webAuthnEnrollConfirmRequest["attestation_response"]!!.jsonObject,
        )
        client.auth.deleteWebAuthn()

        assertEquals("webauthn_enroll_begin_challenge", begin.challenge)
        assertEquals("WebAuthn MFA enrollment confirmed", confirm.message)
        assertEquals("jwt_existing", client.token)
        assertEquals("refresh_existing", client.refreshToken)
        assertFalse(events.contains(AuthStateEvent.SIGNED_IN))

        val enrollRequest = transport.requests[0]
        assertEquals(HttpMethod.POST, enrollRequest.method)
        assertEquals("/api/auth/mfa/webauthn/enroll", java.net.URI(enrollRequest.url).path)
        assertEquals("Bearer jwt_existing", lowercasedLookup(enrollRequest.headers, "authorization"))
        assertNull(enrollRequest.body)

        val confirmRequest = transport.requests[1]
        assertEquals(HttpMethod.POST, confirmRequest.method)
        assertEquals("/api/auth/mfa/webauthn/enroll/confirm", java.net.URI(confirmRequest.url).path)
        assertEquals("Bearer jwt_existing", lowercasedLookup(confirmRequest.headers, "authorization"))
        assertEquals(
            ContractFixtures.webAuthnEnrollConfirmRequest,
            json.parseToJsonElement(confirmRequest.body!!.decodeToString()).jsonObject,
        )

        val deleteRequest = transport.requests[2]
        assertEquals(HttpMethod.DELETE, deleteRequest.method)
        assertEquals("/api/auth/mfa/webauthn", java.net.URI(deleteRequest.url).path)
        assertEquals("Bearer jwt_existing", lowercasedLookup(deleteRequest.headers, "authorization"))
        assertNull(deleteRequest.body)
    }

    @Test
    fun `webauthn mfa challenge and verify use mfa bearer and verify signs in`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = ContractFixtures.webAuthnMfaChallengeResponse))
        transport.enqueue(StubResponse(status = 200, json = ContractFixtures.webAuthnMfaVerifyResponse))
        val client = AYBClient("https://api.example.com", transport = transport)
        client.setTokens("jwt_existing", "refresh_existing")
        val events = mutableListOf<AuthStateEvent>()
        client.onAuthStateChange { event, _ -> events.add(event) }

        val challenge = client.auth.webauthnChallenge("mfa_pending_token")
        val response = client.auth.webauthnVerify(
            "mfa_pending_token",
            "webauthn_mfa_challenge_fixture",
            ContractFixtures.webAuthnMfaVerifyRequest["assertion_response"]!!.jsonObject,
        )

        assertEquals("webauthn_mfa_challenge_fixture", challenge.challengeId)
        assertEquals("jwt_webauthn_mfa", response.token)
        assertEquals("jwt_webauthn_mfa", client.token)
        assertEquals("refresh_webauthn_mfa", client.refreshToken)
        assertEquals(listOf(AuthStateEvent.SIGNED_IN), events)

        val challengeRequest = transport.requests[0]
        assertEquals(HttpMethod.POST, challengeRequest.method)
        assertEquals("/api/auth/mfa/webauthn/challenge", java.net.URI(challengeRequest.url).path)
        assertEquals("Bearer mfa_pending_token", lowercasedLookup(challengeRequest.headers, "authorization"))
        assertNull(challengeRequest.body)

        val verifyRequest = transport.requests[1]
        assertEquals(HttpMethod.POST, verifyRequest.method)
        assertEquals("/api/auth/mfa/webauthn/verify", java.net.URI(verifyRequest.url).path)
        assertEquals("Bearer mfa_pending_token", lowercasedLookup(verifyRequest.headers, "authorization"))
        assertEquals(
            ContractFixtures.webAuthnMfaVerifyRequest,
            json.parseToJsonElement(verifyRequest.body!!.decodeToString()).jsonObject,
        )
    }

    @Test
    fun `confirm magic link stores tokens for authenticated response`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = ContractFixtures.magicLinkConfirmSuccessResponse))
        val client = AYBClient("https://api.example.com", transport = transport)
        val events = mutableListOf<AuthStateEvent>()
        client.onAuthStateChange { event, _ -> events.add(event) }

        val response = client.auth.confirmMagicLink("sdk-parity-magic-token")

        when (response) {
            is MagicLinkConfirmResponse.Authenticated -> {
                assertEquals("magic@allyourbase.io", response.auth.user.email)
                assertEquals("jwt_magic_link", client.token)
                assertEquals("refresh_magic_link", client.refreshToken)
                assertEquals(listOf(AuthStateEvent.SIGNED_IN), events)
            }
            is MagicLinkConfirmResponse.PendingMfa -> throw AssertionError("expected authenticated magic-link response")
        }

        val request = transport.requests.single()
        assertEquals("/api/auth/magic-link/confirm", java.net.URI(request.url).path)
        val body = json.parseToJsonElement(request.body!!.decodeToString()) as JsonObject
        assertEquals("sdk-parity-magic-token", body["token"]!!.jsonPrimitive.content)
    }

    @Test
    fun `confirm magic link pending mfa preserves existing tokens and emits no signed in`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = ContractFixtures.magicLinkConfirmPendingMfaResponse))
        val client = AYBClient("https://api.example.com", transport = transport)
        client.setTokens("jwt_existing", "refresh_existing")
        val events = mutableListOf<AuthStateEvent>()
        client.onAuthStateChange { event, _ -> events.add(event) }

        val response = client.auth.confirmMagicLink("pending-token")

        when (response) {
            is MagicLinkConfirmResponse.PendingMfa -> {
                assertEquals("mfa_pending_token_stage1", response.mfaToken)
                assertEquals("jwt_existing", client.token)
                assertEquals("refresh_existing", client.refreshToken)
                assertFalse(events.contains(AuthStateEvent.SIGNED_IN))
            }
            is MagicLinkConfirmResponse.Authenticated -> throw AssertionError("expected pending mfa response")
        }
    }

    @Test
    fun `confirm magic link non 2xx propagates AYBException from response`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(
            StubResponse(
                status = 401,
                json = buildJsonObject {
                    put("code", "auth/invalid-magic-link")
                    put("message", "invalid magic link token")
                },
            ),
        )
        val client = AYBClient("https://api.example.com", transport = transport)

        runCatching { client.auth.confirmMagicLink("bad-token") }
            .onSuccess { throw AssertionError("expected confirmMagicLink failure") }
            .onFailure { error ->
                val ayb = error as AYBException
                assertEquals(401, ayb.status)
                assertEquals("auth/invalid-magic-link", ayb.code)
                assertEquals("invalid magic link token", ayb.message)
            }
    }

    @Test
    fun `link email uses authenticated request and returns linked user`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = ContractFixtures.linkEmailResponse))
        val client = AYBClient("https://api.example.com", transport = transport)
        client.setTokens("anon_token", "anon_refresh")

        val response = client.auth.linkEmail("upgraded@example.com", "LinkedPass123!")

        assertEquals("upgraded@example.com", response.user.email)
        assertFalse(response.user.isAnonymous ?: false)
        assertNotNull(response.user.linkedAt)
        assertEquals(response.token, client.token)
        assertEquals(response.refreshToken, client.refreshToken)

        val request = transport.requests.single()
        assertEquals("Bearer anon_token", lowercasedLookup(request.headers, "authorization"))
        val body = json.parseToJsonElement(request.body!!.decodeToString()) as JsonObject
        assertEquals("upgraded@example.com", body["email"]!!.jsonPrimitive.content)
        assertEquals("LinkedPass123!", body["password"]!!.jsonPrimitive.content)
    }

    @Test
    fun `refresh posts refresh token stores new tokens and emits event`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = authResponseJson("jwt_new", "refresh_new")))
        val client = AYBClient("https://api.example.com", transport = transport)
        client.setTokens("jwt_old", "refresh_old")

        val events = mutableListOf<AuthStateEvent>()
        client.onAuthStateChange { event, _ -> events.add(event) }

        val refreshed = client.auth.refresh()

        assertEquals("jwt_new", refreshed.token)
        assertEquals("jwt_new", client.token)
        assertEquals("refresh_new", client.refreshToken)
        val body = json.parseToJsonElement(transport.requests.first().body!!.decodeToString()) as JsonObject
        assertEquals("refresh_old", body["refreshToken"]!!.jsonPrimitive.content)
        assertTrue(events.contains(AuthStateEvent.TOKEN_REFRESHED))
    }

    @Test
    fun `logout clears tokens and emits signed out`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 204, body = ByteArray(0)))
        val client = AYBClient("https://api.example.com", transport = transport)
        client.setTokens("jwt_old", "refresh_old")

        val events = mutableListOf<AuthStateEvent>()
        client.onAuthStateChange { event, _ -> events.add(event) }

        client.auth.logout()

        assertNull(client.token)
        assertNull(client.refreshToken)
        assertTrue(events.contains(AuthStateEvent.SIGNED_OUT))
    }

    @Test
    fun `missing refresh token throws without network call`() = runTest {
        val transport = MockHttpTransport()
        val client = AYBClient("https://api.example.com", transport = transport)

        runCatching { client.auth.refresh() }
            .onSuccess { throw AssertionError("expected failure") }
            .onFailure { error ->
                val ayb = error as AYBException
                assertEquals("auth/missing-refresh-token", ayb.code)
            }

        assertEquals(0, transport.requests.size)
    }

    @Test
    fun `token lifecycle login to logout clears state`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = authResponseJson("jwt_login", "refresh_login")))
        transport.enqueue(StubResponse(status = 204, body = ByteArray(0)))

        val client = AYBClient("https://api.example.com", transport = transport)

        client.auth.login("dev@allyourbase.io", "secret")
        assertEquals("jwt_login", client.token)
        assertEquals("refresh_login", client.refreshToken)

        client.auth.logout()
        assertNull(client.token)
        assertNull(client.refreshToken)
    }

    @Test
    fun `auth state emits signed in and token refreshed and supports unsubscribe`() = runTest {
        val transport = MockHttpTransport()
        transport.enqueue(StubResponse(status = 200, json = authResponseJson("jwt_login", "refresh_login")))
        transport.enqueue(StubResponse(status = 200, json = authResponseJson("jwt_refresh", "refresh_refresh")))

        val client = AYBClient("https://api.example.com", transport = transport)
        val events = mutableListOf<AuthStateEvent>()

        val unsubscribe = client.onAuthStateChange { event, _ -> events.add(event) }
        client.auth.login("dev@allyourbase.io", "secret")
        client.auth.refresh()

        unsubscribe()
        client.emitAuthState(AuthStateEvent.SIGNED_OUT)

        assertEquals(listOf(AuthStateEvent.SIGNED_IN, AuthStateEvent.TOKEN_REFRESHED), events)
    }

    private fun authResponseJson(token: String, refresh: String) = buildJsonObject {
        put("token", token)
        put("refreshToken", refresh)
        putJsonObject("user") {
            put("id", "usr_1")
            put("email", "dev@allyourbase.io")
            put("email_verified", true)
            put("created_at", "2026-01-01T00:00:00Z")
        }
    }

    private fun buildPasskeyAssertionResponse() = buildJsonObject {
        put("id", "credential-id")
        put("rawId", "credential-id")
        put("type", "public-key")
        putJsonObject("response") {
            put("clientDataJSON", "client-data-json")
            put("authenticatorData", "authenticator-data")
            put("signature", "signature")
            put("userHandle", "user-handle")
        }
    }
}
