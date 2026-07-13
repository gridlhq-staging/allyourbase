import Foundation
import Testing
@testable import Allyourbase

struct WebAuthnLoginTests {
    @Test func mfaChallengeFixtureDecodesCanonicalContract() throws {
        let response = try WebAuthnMFAChallengeResponse.decode(ContractFixtures.webAuthnMFAChallengeResponse)
        let firstCredential = try #require(response.options.allowCredentials.first)

        #expect(response.challengeId == "webauthn_mfa_challenge_fixture")
        #expect(response.options.challenge == "webauthn_mfa_challenge")
        #expect(response.options.rpId == "127.0.0.1")
        #expect(response.options.timeout == 300000)
        #expect(response.options.allowCredentials.count == 1)
        #expect(firstCredential.id == "webauthn_mfa_credential_a")
        #expect(firstCredential.type == "public-key")
    }

    @Test func loginBeginFixtureDecodesCanonicalContract() throws {
        let response = try WebAuthnLoginBeginResponse.decode(ContractFixtures.webAuthnLoginBeginResponse)
        let firstCredential = try #require(response.options.allowCredentials.first)

        #expect(response.challengeId == "webauthn_challenge_fixture")
        #expect(response.options.challenge == "webauthn_login_begin_challenge")
        #expect(response.options.rpId == "127.0.0.1")
        #expect(response.options.timeout == 300000)
        #expect(response.options.allowCredentials.count == 1)
        #expect(firstCredential.id == "webauthn_login_begin_credential_a")
        #expect(firstCredential.type == "public-key")
    }

    @Test func beginWebAuthnLoginPostsEmailWithoutMutatingTokens() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.webAuthnLoginBeginResponse))
        let tokenStore = InMemoryTokenStore(accessToken: "jwt_existing", refreshToken: "refresh_existing")
        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: tokenStore
        )

        let response = try await client.auth.beginWebAuthnLogin(email: "passkey@example.com")

        #expect(response.challengeId == "webauthn_challenge_fixture")
        #expect(client.token == "jwt_existing")
        #expect(client.refreshToken == "refresh_existing")
        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/webauthn/login/begin")
        #expect(request.method.rawValue == "POST")
        let body = try #require(request.body)
        let payload = try #require(JSONSerialization.jsonObject(with: body) as? [String: String])
        #expect(payload == ["email": "passkey@example.com"])
    }

    @Test func finishWebAuthnLoginPostsAssertionAndStoresAuthSession() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.authResponse))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)
        var emitted: [AuthStateEvent] = []
        _ = client.onAuthStateChange { event, _ in emitted.append(event) }
        let assertionResponse: [String: Any] = [
            "id": "credential-id",
            "rawId": "credential-raw-id",
            "response": [
                "authenticatorData": "auth-data",
                "clientDataJSON": "client-json",
                "signature": "signature",
            ],
            "type": "public-key",
        ]

        let response = try await client.auth.finishWebAuthnLogin(
            challengeId: "webauthn_challenge_fixture",
            assertionResponse: assertionResponse
        )

        #expect(response.token == "jwt_stage3")
        #expect(response.refreshToken == "refresh_stage3")
        #expect(client.token == "jwt_stage3")
        #expect(client.refreshToken == "refresh_stage3")
        #expect(emitted == [.signedIn])
        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/webauthn/login/finish")
        #expect(request.method.rawValue == "POST")
        let body = try #require(request.body)
        let payload = try #require(JSONSerialization.jsonObject(with: body) as? [String: Any])
        #expect(payload["challenge_id"] as? String == "webauthn_challenge_fixture")
        #expect(payload["challengeId"] == nil)
        #expect(payload["assertion_response"] != nil)
        #expect(payload["assertionResponse"] == nil)
        let encodedAssertion = try #require(payload["assertion_response"] as? [String: Any])
        #expect(encodedAssertion["id"] as? String == "credential-id")
        #expect(encodedAssertion["type"] as? String == "public-key")
    }

    @Test func beginDiscoverableWebAuthnLoginPostsBodylessRequestWithoutSessionBearer() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.webAuthnDiscoverBeginResponse))
        let tokenStore = InMemoryTokenStore(accessToken: "existing_jwt", refreshToken: "existing_refresh")
        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: tokenStore
        )

        let response = try await client.auth.beginDiscoverableWebAuthnLogin()

        #expect(response.challengeId == "webauthn_discover_challenge_fixture")
        #expect(response.options.challenge == "webauthn_discover_challenge")
        #expect(response.options.rpId == "127.0.0.1")
        #expect(response.options.timeout == 300000)
        #expect(response.options.allowCredentials.isEmpty == true)
        #expect(client.token == "existing_jwt")
        #expect(client.refreshToken == "existing_refresh")
        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/webauthn/login/discover/begin")
        #expect(request.method.rawValue == "POST")
        #expect(request.body == nil)
        #expect(lowercasedLookup(request.headers, "Authorization") == nil)
    }

    @Test func finishDiscoverableWebAuthnLoginPostsSnakeCasePayloadAndStoresSession() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.authResponse))
        let tokenStore = InMemoryTokenStore(accessToken: "existing_jwt", refreshToken: "existing_refresh")
        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: tokenStore
        )
        var emitted: [AuthStateEvent] = []
        _ = client.onAuthStateChange { event, _ in emitted.append(event) }
        let finishFixture = ContractFixtures.webAuthnDiscoverFinishRequest
        let assertionResponse = try #require(finishFixture["assertion_response"] as? [String: Any])

        let response = try await client.auth.finishDiscoverableWebAuthnLogin(
            challengeId: "webauthn_discover_challenge_fixture",
            assertionResponse: assertionResponse
        )

        #expect(response.token == "jwt_stage3")
        #expect(response.refreshToken == "refresh_stage3")
        #expect(client.token == "jwt_stage3")
        #expect(client.refreshToken == "refresh_stage3")
        #expect(emitted == [.signedIn])
        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/webauthn/login/discover/finish")
        #expect(request.method.rawValue == "POST")
        #expect(lowercasedLookup(request.headers, "Authorization") == nil)
        let body = try #require(request.body)
        let payload = try #require(JSONSerialization.jsonObject(with: body) as? [String: Any])
        #expect(payload["challenge_id"] as? String == "webauthn_discover_challenge_fixture")
        #expect(payload["challengeId"] == nil)
        #expect(payload["assertionResponse"] == nil)
        let encodedAssertion = try #require(payload["assertion_response"] as? [String: Any])
        #expect(encodedAssertion["id"] as? String == "webauthn_discover_credential")
        #expect(encodedAssertion["type"] as? String == "public-key")
    }

    @Test func enrollWebAuthnUsesCurrentSessionAndDecodesCreationOptions() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.webAuthnEnrollBeginResponse))
        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: InMemoryTokenStore(accessToken: "jwt_current", refreshToken: "refresh_current")
        )

        let response = try await client.auth.enrollWebAuthn()

        #expect(response.challenge == "webauthn_enroll_begin_challenge")
        #expect(client.token == "jwt_current")
        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/mfa/webauthn/enroll")
        #expect(request.method.rawValue == "POST")
        #expect(lowercasedLookup(request.headers, "Authorization") == "Bearer jwt_current")
    }

    @Test func confirmWebAuthnEnrollmentPostsCanonicalAttestationBody() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.webAuthnEnrollConfirmResponse))
        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: InMemoryTokenStore(accessToken: "jwt_current", refreshToken: "refresh_current")
        )
        let requestModel = try WebAuthnEnrollConfirmRequest(from: ContractFixtures.webAuthnEnrollConfirmRequest)

        let response = try await client.auth.confirmWebAuthnEnrollment(requestModel)

        #expect(response.message == "WebAuthn MFA enrollment confirmed")
        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/mfa/webauthn/enroll/confirm")
        #expect(request.method.rawValue == "POST")
        #expect(lowercasedLookup(request.headers, "Authorization") == "Bearer jwt_current")
        let body = try #require(request.body)
        let payload = try #require(JSONSerialization.jsonObject(with: body) as? [String: Any])
        #expect(payload["display_name"] as? String == "Primary security key")
        #expect(payload["displayName"] == nil)
        #expect(payload["attestation_response"] != nil)
        #expect(payload["attestationResponse"] == nil)
    }

    @Test func webAuthnMFAChallengeUsesExplicitMFATokenWithoutMutatingStoredSession() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.webAuthnMFAChallengeResponse))
        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: InMemoryTokenStore(accessToken: "jwt_current", refreshToken: "refresh_current")
        )

        let response = try await client.auth.webauthnChallenge(mfaToken: "mfa_pending_token")

        #expect(response.challengeId == "webauthn_mfa_challenge_fixture")
        #expect(client.token == "jwt_current")
        #expect(client.refreshToken == "refresh_current")
        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/mfa/webauthn/challenge")
        #expect(request.method.rawValue == "POST")
        #expect(lowercasedLookup(request.headers, "Authorization") == "Bearer mfa_pending_token")
    }

    @Test func webAuthnMFAVerifyUsesExplicitMFATokenAndStoresSignedInSession() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.webAuthnMFAVerifyResponse))
        let tokenStore = InMemoryTokenStore(accessToken: "jwt_current", refreshToken: "refresh_current")
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport, tokenStore: tokenStore)
        var emitted: [AuthStateEvent] = []
        _ = client.onAuthStateChange { event, _ in emitted.append(event) }
        let verifyRequest = try WebAuthnMFAVerifyRequest(from: ContractFixtures.webAuthnMFAVerifyRequest)

        let response = try await client.auth.webauthnVerify(
            mfaToken: "mfa_pending_token",
            challengeId: verifyRequest.challengeId,
            assertionResponse: verifyRequest.assertionResponse.toDictionary()
        )

        #expect(response.token == "jwt_webauthn_mfa")
        #expect(client.token == "jwt_webauthn_mfa")
        #expect(client.refreshToken == "refresh_webauthn_mfa")
        #expect(emitted == [.signedIn])
        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/mfa/webauthn/verify")
        #expect(request.method.rawValue == "POST")
        #expect(lowercasedLookup(request.headers, "Authorization") == "Bearer mfa_pending_token")
        let body = try #require(request.body)
        let payload = try #require(JSONSerialization.jsonObject(with: body) as? [String: Any])
        #expect(payload["challenge_id"] as? String == "webauthn_mfa_challenge_fixture")
        #expect(payload["challengeId"] == nil)
        #expect(payload["assertion_response"] != nil)
        #expect(payload["assertionResponse"] == nil)
    }

    @Test func deleteWebAuthnUsesCurrentSession() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 204, json: NSNull()))
        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: InMemoryTokenStore(accessToken: "jwt_current", refreshToken: "refresh_current")
        )

        try await client.auth.deleteWebAuthn()

        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/mfa/webauthn/")
        #expect(request.method.rawValue == "DELETE")
        #expect(lowercasedLookup(request.headers, "Authorization") == "Bearer jwt_current")
        #expect(client.token == "jwt_current")
    }
}
