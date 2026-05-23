import Foundation
import Testing
@testable import Allyourbase

struct AuthClientTests {
    @Test func registerGeneratesExpectedRequest() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.authResponse))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        _ = try await client.auth.register(email: "test@example.com", password: "secret123")

        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/register")
        #expect(request.method.rawValue == "POST")

        let body = try #require(request.body)
        let payload = try #require(JSONSerialization.jsonObject(with: body) as? [String: String])
        #expect(payload["email"] == "test@example.com")
        #expect(payload["password"] == "secret123")
        #expect(lowercasedLookup(request.headers, "content-type") == "application/json")
    }

    @Test func loginGeneratesExpectedRequest() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.authResponse))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        _ = try await client.auth.login(email: "test@example.com", password: "secret123")

        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/login")
        #expect(request.method.rawValue == "POST")
    }

    @Test func signInAnonymouslyStoresTokensAndEmitsSignedIn() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 201, json: ContractFixtures.anonymousAuthResponse))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        var emitted: [AuthStateEvent] = []
        _ = client.onAuthStateChange { event, _ in emitted.append(event) }

        let response = try await client.auth.signInAnonymously()

        #expect(response.user.isAnonymous == true)
        #expect(client.token == response.token)
        #expect(client.refreshToken == response.refreshToken)
        #expect(emitted == [.signedIn])
        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/anonymous")
        #expect(request.method.rawValue == "POST")
    }

    @Test func requestMagicLinkPostsEmailWithoutMutatingTokens() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.magicLinkRequestResponse))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        let response = try await client.auth.requestMagicLink(email: "fixture@example.com")

        #expect(response.message == "If an account exists, a magic link has been sent.")
        #expect(client.token == nil)
        #expect(client.refreshToken == nil)
        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/magic-link")
        let body = try #require(request.body)
        let payload = try #require(JSONSerialization.jsonObject(with: body) as? [String: String])
        #expect(payload["email"] == "fixture@example.com")
    }

    @Test func confirmMagicLinkStoresTokensForAuthenticatedResponse() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.magicLinkConfirmResponse))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)
        var emitted: [AuthStateEvent] = []
        _ = client.onAuthStateChange { event, _ in emitted.append(event) }

        let response = try await client.auth.confirmMagicLink(token: "sdk-parity-magic-token")

        switch response {
        case .authenticated(let auth):
            #expect(auth.user.email == "magic@allyourbase.io")
            #expect(client.token == "jwt_magic_link")
            #expect(client.refreshToken == "refresh_magic_link")
            #expect(emitted == [.signedIn])
        case .pendingMFA:
            Issue.record("expected authenticated magic-link response")
        }
    }

    @Test func confirmMagicLinkReturnsPendingMFAWithoutMutatingTokens() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.magicLinkConfirmPendingMFAResponse))
        let tokenStore = InMemoryTokenStore(accessToken: "jwt_existing", refreshToken: "refresh_existing")
        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: tokenStore
        )
        var emitted: [AuthStateEvent] = []
        _ = client.onAuthStateChange { event, _ in emitted.append(event) }

        let response = try await client.auth.confirmMagicLink(token: "pending-mfa-token")

        switch response {
        case .pendingMFA(let mfaToken):
            #expect(mfaToken == "mfa_pending_token_stage1")
            #expect(client.token == "jwt_existing")
            #expect(client.refreshToken == "refresh_existing")
            #expect(!emitted.contains(.signedIn))
        case .authenticated:
            Issue.record("expected pending MFA response")
        }
    }

    @Test func confirmMagicLinkPropagatesNon2xxAYBError() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 401, json: [
            "code": "auth/invalid-magic-link",
            "message": "invalid magic link token",
        ]))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        do {
            _ = try await client.auth.confirmMagicLink(token: "bad-token")
            Issue.record("expected confirmMagicLink to fail")
        } catch let error as AYBError {
            #expect(error.status == 401)
            #expect(error.code == "auth/invalid-magic-link")
            #expect(error.message == "invalid magic link token")
        }
    }

    @Test func linkEmailUsesAuthenticatedRequestAndReturnsLinkedUser() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.linkEmailResponse))
        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: InMemoryTokenStore(accessToken: "anon_token", refreshToken: "anon_refresh")
        )

        let response = try await client.auth.linkEmail(email: "upgraded@example.com", password: "LinkedPass123!")
        let fixtureUser = try #require(ContractFixtures.linkEmailResponse["user"] as? [String: Any])
        let fixtureLinkedAt = try #require(fixtureUser["linked_at"] as? String)
        let fixtureToken = try #require(ContractFixtures.linkEmailResponse["token"] as? String)
        let fixtureRefreshToken = try #require(ContractFixtures.linkEmailResponse["refreshToken"] as? String)

        #expect(response.user.email == "upgraded@example.com")
        #expect(response.user.isAnonymous == nil)
        #expect(response.user.linkedAt == fixtureLinkedAt)
        #expect(response.token == fixtureToken)
        #expect(response.refreshToken == fixtureRefreshToken)
        #expect(client.token == response.token)
        #expect(client.refreshToken == response.refreshToken)
        let request = try #require(transport.requests.last)
        #expect(lowercasedLookup(request.headers, "Authorization") == "Bearer anon_token")
        let body = try #require(request.body)
        let payload = try #require(JSONSerialization.jsonObject(with: body) as? [String: String])
        #expect(payload["email"] == "upgraded@example.com")
        #expect(payload["password"] == "LinkedPass123!")
    }

    @Test func meUsesAuthTokenWhenAvailable() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: [
            "id": "usr_1",
            "email": "test@example.com",
        ]))

        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: InMemoryTokenStore(accessToken: "jwt_abc", refreshToken: "refresh_abc")
        )

        _ = try await client.auth.me()

        let request = try #require(transport.requests.last)
        #expect(request.method.rawValue == "GET")
        #expect(lowercasedLookup(request.headers, "Authorization") == "Bearer jwt_abc")
    }

    @Test func refreshPostsRefreshTokenAndStoresNewTokens() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.authResponse))

        let tokenStore = InMemoryTokenStore(accessToken: "jwt_old", refreshToken: "refresh_old")
        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: tokenStore
        )

        let response = try await client.auth.refresh()

        #expect(response.token == "jwt_stage3")
        #expect(response.refreshToken == "refresh_stage3")
        #expect(client.token == "jwt_stage3")
        #expect(client.refreshToken == "refresh_stage3")
        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/refresh")
        #expect(lowercasedLookup(request.headers, "Authorization") == "Bearer jwt_old")
        let payload = try JSONSerialization.jsonObject(with: try #require(request.body), options: []) as? [String: String]
        #expect(payload?["refreshToken"] == "refresh_old")
    }

    @Test func logoutClearsTokensAndEmitsSignedOut() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 204, json: NSNull()))
        let tokenStore = InMemoryTokenStore(accessToken: "jwt_abc", refreshToken: "refresh_abc")
        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: tokenStore
        )

        var emitted: [AuthStateEvent] = []
        _ = client.onAuthStateChange { event, _ in emitted.append(event) }

        try await client.auth.logout()

        #expect(client.token == nil)
        #expect(client.refreshToken == nil)
        #expect(emitted == [.signedOut])
        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/logout")
        #expect(request.method.rawValue == "POST")
        #expect(lowercasedLookup(request.headers, "Authorization") == "Bearer jwt_abc")
    }

    @Test func missingRefreshTokenSkipsNetwork() async {
        let transport = MockTransport()
        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: InMemoryTokenStore()
        )

        do {
            _ = try await client.auth.refresh()
            Issue.record("refresh should fail without refresh token")
        } catch let error as AYBError {
            #expect(error.status == 400)
            #expect(error.code == "auth/missing-refresh-token")
            #expect(transport.requests.isEmpty)
        } catch {
            Issue.record("unexpected error type: \(error)")
        }
    }

    @Test func tokenLifecycleForSignupAndSignout() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.authResponse))
        transport.enqueue(StubResponse(status: 204, json: NSNull()))
        transport.enqueue(StubResponse(status: 200, json: ["id": "usr_1", "email": "test@example.com"]))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        var sessionEvents: [AuthStateEvent] = []
        _ = client.onAuthStateChange { event, _ in sessionEvents.append(event) }

        _ = try await client.auth.login(email: "test@example.com", password: "secret123")
        #expect(client.token == "jwt_stage3")

        try await client.auth.logout()
        // requests[1] is the logout request, which should carry the jwt_stage3 token acquired at login
        assertTransportRequestHasAuthorization(transport.requests[1], token: "jwt_stage3")
        #expect(client.token == nil)
        #expect(client.refreshToken == nil)
        #expect(sessionEvents == [.signedIn, .signedOut])

        _ = try await client.auth.me()
        let lastRequest = try #require(transport.requests.last)
        #expect(lowercasedLookup(lastRequest.headers, "Authorization") == nil)
    }

    @Test func signInEmitsSignedInAndRefreshEmitsTokenRefreshed() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.authResponse))
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.authResponse))
        let tokenStore = InMemoryTokenStore(accessToken: "jwt_init", refreshToken: "refresh_init")
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport, tokenStore: tokenStore)

        var events: [AuthStateEvent] = []
        _ = client.onAuthStateChange { event, _ in
            events.append(event)
        }

        _ = try await client.auth.login(email: "test@example.com", password: "secret")
        _ = try await client.auth.refresh()

        #expect(events == [.signedIn, .tokenRefreshed])
    }

    @Test func unsubscribeAuthListenerDuringEmit() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.authResponse))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        var observed: [AuthStateEvent] = []
        var unsubscribe: (() -> Void)?

        unsubscribe = client.onAuthStateChange { event, _ in
            observed.append(event)
            unsubscribe?()
        }

        _ = try await client.auth.login(email: "test@example.com", password: "secret")

        #expect(observed == [.signedIn])
    }
}

private func assertTransportRequestHasAuthorization(_ request: HTTPRequest, token: String) {
    #expect(lowercasedLookup(request.headers, "Authorization") == "Bearer \(token)")
}
