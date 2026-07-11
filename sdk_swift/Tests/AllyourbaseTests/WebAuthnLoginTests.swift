import Foundation
import Testing
@testable import Allyourbase

struct WebAuthnLoginTests {
    @Test func loginBeginFixtureDecodesCanonicalContract() throws {
        let response = try WebAuthnLoginBeginResponse.decode(ContractFixtures.webAuthnLoginBeginResponse)
        let firstCredential = try #require(response.options.allowCredentials.first)

        #expect(response.challengeId == "webauthn_challenge_fixture")
        #expect(response.options.challenge == "webauthn_login_begin_challenge")
        #expect(response.options.rpId == "127.0.0.1")
        #expect(response.options.timeout == 300000)
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
}
