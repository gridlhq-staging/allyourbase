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
