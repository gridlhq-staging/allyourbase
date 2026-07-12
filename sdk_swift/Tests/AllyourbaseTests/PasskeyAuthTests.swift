import Foundation
#if canImport(AuthenticationServices)
import AuthenticationServices
#endif
import Testing
@testable import Allyourbase

struct PasskeyAuthTests {
    @Test func signInWithPasskeyRunsBeginAuthenticatorAndFinishFlow() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: passkeyBeginResponse()))
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.authResponse))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)
        let authenticator = FakePasskeyAuthenticator(assertionResponse: .init(
            id: "Y3JlZGVudGlhbC1pZA",
            rawId: "Y3JlZGVudGlhbC1yYXctaWQ",
            response: .init(
                clientDataJSON: "Y2xpZW50LWpzb24",
                authenticatorData: "YXV0aC1kYXRh",
                signature: "c2lnbmF0dXJl",
                userHandle: "dXNlci1oYW5kbGU"
            )
        ))
        var emitted: [AuthStateEvent] = []
        _ = client.onAuthStateChange { event, _ in emitted.append(event) }

        let response = try await client.auth.signInWithPasskey(
            email: "passkey@example.com",
            authenticator: authenticator
        )

        #expect(response.token == "jwt_stage3")
        #expect(client.token == "jwt_stage3")
        #expect(client.refreshToken == "refresh_stage3")
        #expect(emitted == [.signedIn])
        #expect(authenticator.capturedOptions.count == 1)
        let capturedOptions = try #require(authenticator.capturedOptions.first)
        #expect(capturedOptions.challenge == "Y2hhbGxlbmdlLWJ5dGVz")
        #expect(capturedOptions.timeout == 120000)
        #expect(capturedOptions.rpId == "login.example.com")
        #expect(capturedOptions.userVerification == "required")
        #expect(capturedOptions.allowCredentials.map(\.id) == [
            "Y3JlZGVudGlhbC1h",
            "Y3JlZGVudGlhbC1i",
        ])
        #expect(capturedOptions.allowCredentials.map(\.type) == [
            "public-key",
            "public-key",
        ])

        #expect(transport.requests.count == 2)
        let beginRequest = transport.requests[0]
        #expect(beginRequest.url.absoluteString == "https://api.example.com/api/auth/webauthn/login/begin")
        #expect(beginRequest.method.rawValue == "POST")
        let beginBody = try #require(beginRequest.body)
        let beginPayload = try #require(JSONSerialization.jsonObject(with: beginBody) as? [String: String])
        #expect(beginPayload == ["email": "passkey@example.com"])

        let finishRequest = transport.requests[1]
        #expect(finishRequest.url.absoluteString == "https://api.example.com/api/auth/webauthn/login/finish")
        #expect(finishRequest.method.rawValue == "POST")
        let finishBody = try #require(finishRequest.body)
        let payload = try #require(JSONSerialization.jsonObject(with: finishBody) as? [String: Any])
        #expect(payload["challenge_id"] as? String == "challenge-original-id")
        #expect(payload["challengeId"] == nil)
        let assertion = try #require(payload["assertion_response"] as? [String: Any])
        #expect(payload["assertionResponse"] == nil)
        #expect(assertion["id"] as? String == "Y3JlZGVudGlhbC1pZA")
        #expect(assertion["rawId"] as? String == "Y3JlZGVudGlhbC1yYXctaWQ")
        #expect(assertion["type"] as? String == "public-key")
        let assertionResponse = try #require(assertion["response"] as? [String: Any])
        #expect(assertionResponse["clientDataJSON"] as? String == "Y2xpZW50LWpzb24")
        #expect(assertionResponse["authenticatorData"] as? String == "YXV0aC1kYXRh")
        #expect(assertionResponse["signature"] as? String == "c2lnbmF0dXJl")
        #expect(assertionResponse["userHandle"] as? String == "dXNlci1oYW5kbGU")
        let clientExtensionResults = try #require(assertion["clientExtensionResults"] as? [String: Any])
        #expect(clientExtensionResults.isEmpty)
    }

    #if canImport(AuthenticationServices)
    @Test @MainActor func systemPasskeyAuthenticatorBuildsAssertionRequestFromOptions() throws {
        let authenticator = SystemPasskeyAuthenticator()
        let options = PasskeyAssertionRequestOptions(
            challenge: "Y2hhbGxlbmdlLWJ5dGVz",
            timeout: 300000,
            rpId: "example.com",
            allowCredentials: [
                .init(
                    id: "Y3JlZGVudGlhbC1pZA",
                    type: "public-key"
                )
            ],
            userVerification: "required"
        )

        let request = try authenticator.makeAssertionRequest(options: options)

        #expect(request.relyingPartyIdentifier == "example.com")
        #expect(request.challenge == Data("challenge-bytes".utf8))
        #expect(request.userVerificationPreference == .required)
        #expect(request.allowedCredentials.count == 1)
        let firstCredential = try #require(request.allowedCredentials.first)
        #expect(firstCredential.credentialID == Data("credential-id".utf8))
    }

    @Test @MainActor func systemPasskeyAuthenticatorMapsUserVerificationBranches() throws {
        let authenticator = SystemPasskeyAuthenticator()

        let requiredRequest = try authenticator.makeAssertionRequest(
            options: requestOptions(userVerification: "required")
        )
        let discouragedRequest = try authenticator.makeAssertionRequest(
            options: requestOptions(userVerification: "discouraged")
        )
        let preferredRequest = try authenticator.makeAssertionRequest(
            options: requestOptions(userVerification: "preferred")
        )
        let defaultRequest = try authenticator.makeAssertionRequest(
            options: requestOptions(userVerification: nil)
        )

        #expect(requiredRequest.userVerificationPreference == .required)
        #expect(discouragedRequest.userVerificationPreference == .discouraged)
        #expect(preferredRequest.userVerificationPreference == .preferred)
        #expect(defaultRequest.userVerificationPreference == .preferred)
    }

    @Test func systemPasskeyAuthenticatorSerializesPlatformAssertionForFinishPayload() throws {
        let authenticator = SystemPasskeyAuthenticator()
        let assertion = FakePlatformAssertion(
            rawClientDataJSON: Data("client-json".utf8),
            credentialID: Data("credential-id".utf8),
            rawAuthenticatorData: Data("auth-data".utf8),
            userID: Data("user-handle".utf8),
            signature: Data("sig".utf8)
        )

        let payload = authenticator.serializeAssertion(assertion)
        #expect(payload.id == "Y3JlZGVudGlhbC1pZA")
        #expect(payload.rawId == "Y3JlZGVudGlhbC1pZA")
        #expect(payload.type == "public-key")
        #expect(payload.response.clientDataJSON == "Y2xpZW50LWpzb24")
        #expect(payload.response.authenticatorData == "YXV0aC1kYXRh")
        #expect(payload.response.signature == "c2ln")
        #expect(payload.response.userHandle == "dXNlci1oYW5kbGU")
        let dictionary = payload.toDictionary()
        #expect(dictionary["id"] as? String == "Y3JlZGVudGlhbC1pZA")
        #expect(dictionary["rawId"] as? String == "Y3JlZGVudGlhbC1pZA")
        #expect(dictionary["type"] as? String == "public-key")
        let response = try #require(dictionary["response"] as? [String: Any])
        #expect(response["clientDataJSON"] as? String == "Y2xpZW50LWpzb24")
        #expect(response["authenticatorData"] as? String == "YXV0aC1kYXRh")
        #expect(response["signature"] as? String == "c2ln")
        #expect(response["userHandle"] as? String == "dXNlci1oYW5kbGU")
        let clientExtensionResults = try #require(dictionary["clientExtensionResults"] as? [String: Any])
        #expect(clientExtensionResults.isEmpty)
    }
    #endif

    @Test func passkeyAssertionDictionaryUsesNullUserHandleWhenAbsent() throws {
        let payload = PasskeyAssertionResult(
            id: "Y3JlZGVudGlhbC1pZA",
            rawId: "Y3JlZGVudGlhbC1pZA",
            response: .init(
                clientDataJSON: "Y2xpZW50LWpzb24",
                authenticatorData: "YXV0aC1kYXRh",
                signature: "c2ln",
                userHandle: nil
            )
        )

        let dictionary = payload.toDictionary()
        let response = try #require(dictionary["response"] as? [String: Any])
        #expect(response["userHandle"] is NSNull)
        let clientExtensionResults = try #require(dictionary["clientExtensionResults"] as? [String: Any])
        #expect(clientExtensionResults.isEmpty)
    }

    @Test func enrollPasskeyRunsBeginAttestationAndConfirmFlow() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.webAuthnEnrollBeginResponse))
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.webAuthnEnrollConfirmResponse))
        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: InMemoryTokenStore(accessToken: "jwt_current", refreshToken: "refresh_current")
        )
        let authenticator = FakePasskeyAttestationAuthenticator(attestationResponse: .init(
            id: "attestation-id",
            rawId: "attestation-raw-id",
            response: .init(
                attestationObject: "attestation-object",
                clientDataJSON: "client-json",
                transports: ["internal"]
            )
        ))

        let response = try await client.auth.enrollPasskey(
            displayName: "Primary security key",
            authenticator: authenticator
        )

        #expect(response.message == "WebAuthn MFA enrollment confirmed")
        #expect(authenticator.capturedOptions.map { $0.challenge } == ["webauthn_enroll_begin_challenge"])
        #expect(transport.requests.count == 2)
        let confirmRequest = transport.requests[1]
        #expect(confirmRequest.url.absoluteString == "https://api.example.com/api/auth/mfa/webauthn/enroll/confirm")
        let confirmBody = try #require(confirmRequest.body)
        let payload = try #require(JSONSerialization.jsonObject(with: confirmBody) as? [String: Any])
        #expect(payload["display_name"] as? String == "Primary security key")
        let attestation = try #require(payload["attestation_response"] as? [String: Any])
        #expect(attestation["id"] as? String == "attestation-id")
        #expect(payload["attestationResponse"] == nil)
    }

    @Test func verifyPasskeyUsesExplicitMFATokenForChallengeAndVerify() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.webAuthnMFAChallengeResponse))
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.webAuthnMFAVerifyResponse))
        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: InMemoryTokenStore(accessToken: "jwt_current", refreshToken: "refresh_current")
        )
        let authenticator = FakePasskeyAuthenticator(assertionResponse: .init(
            id: "assertion-id",
            rawId: "assertion-raw-id",
            response: .init(
                clientDataJSON: "client-json",
                authenticatorData: "auth-data",
                signature: "signature",
                userHandle: "user-handle"
            )
        ))

        let response = try await client.auth.verifyPasskey(
            mfaToken: "mfa_pending_token",
            authenticator: authenticator
        )

        #expect(response.token == "jwt_webauthn_mfa")
        #expect(client.token == "jwt_webauthn_mfa")
        #expect(authenticator.capturedOptions.map(\.challenge) == ["webauthn_mfa_challenge"])
        #expect(transport.requests.count == 2)
        #expect(lowercasedLookup(transport.requests[0].headers, "Authorization") == "Bearer mfa_pending_token")
        #expect(lowercasedLookup(transport.requests[1].headers, "Authorization") == "Bearer mfa_pending_token")
        let verifyBody = try #require(transport.requests[1].body)
        let payload = try #require(JSONSerialization.jsonObject(with: verifyBody) as? [String: Any])
        #expect(payload["challenge_id"] as? String == "webauthn_mfa_challenge_fixture")
        #expect(payload["challengeId"] == nil)
        #expect(payload["assertion_response"] != nil)
        #expect(payload["assertionResponse"] == nil)
    }
}

private final class FakePasskeyAuthenticator: PasskeyAuthenticating {
    let assertionResponse: PasskeyAssertionResult
    private(set) var capturedOptions: [PasskeyAssertionRequestOptions] = []

    init(assertionResponse: PasskeyAssertionResult) {
        self.assertionResponse = assertionResponse
    }

    func createAssertionResponse(options: PasskeyAssertionRequestOptions) async throws -> PasskeyAssertionResult {
        capturedOptions.append(options)
        return assertionResponse
    }
}

private final class FakePasskeyAttestationAuthenticator: PasskeyAttestationAuthenticating {
    let attestationResponse: PasskeyAttestationResult
    private(set) var capturedOptions: [PasskeyAttestationCreationOptions] = []

    init(attestationResponse: PasskeyAttestationResult) {
        self.attestationResponse = attestationResponse
    }

    func createAttestationResponse(
        options: PasskeyAttestationCreationOptions
    ) async throws -> PasskeyAttestationResult {
        capturedOptions.append(options)
        return attestationResponse
    }
}

private func passkeyBeginResponse() -> [String: Any] {
    [
        "challenge_id": "challenge-original-id",
        "options": [
            "allowCredentials": [
                [
                    "id": "Y3JlZGVudGlhbC1h",
                    "type": "public-key",
                ],
                [
                    "id": "Y3JlZGVudGlhbC1i",
                    "type": "public-key",
                ],
            ],
            "challenge": "Y2hhbGxlbmdlLWJ5dGVz",
            "rpId": "login.example.com",
            "timeout": 120000,
            "userVerification": "required",
        ],
    ]
}

#if canImport(AuthenticationServices)
private func requestOptions(userVerification: String?) -> PasskeyAssertionRequestOptions {
    PasskeyAssertionRequestOptions(
        challenge: "Y2hhbGxlbmdl",
        timeout: 300000,
        rpId: "example.com",
        allowCredentials: [],
        userVerification: userVerification
    )
}

@objc(AYBFakePlatformAssertion)
private final class FakePlatformAssertion: NSObject, ASAuthorizationPublicKeyCredentialAssertion {
    let rawClientDataJSON: Data
    let credentialID: Data
    let rawAuthenticatorData: Data
    let userID: Data
    let signature: Data

    static var supportsSecureCoding: Bool { true }

    init(
        rawClientDataJSON: Data,
        credentialID: Data,
        rawAuthenticatorData: Data,
        userID: Data,
        signature: Data
    ) {
        self.rawClientDataJSON = rawClientDataJSON
        self.credentialID = credentialID
        self.rawAuthenticatorData = rawAuthenticatorData
        self.userID = userID
        self.signature = signature
    }

    required init?(coder: NSCoder) {
        return nil
    }

    func encode(with coder: NSCoder) {}

    func copy(with zone: NSZone? = nil) -> Any {
        FakePlatformAssertion(
            rawClientDataJSON: rawClientDataJSON,
            credentialID: credentialID,
            rawAuthenticatorData: rawAuthenticatorData,
            userID: userID,
            signature: signature
        )
    }
}
#endif
