import Foundation

public struct PasskeyAssertionRequestOptions: Sendable {
    public struct CredentialDescriptor: Sendable {
        public let id: String
        public let type: String

        public init(id: String, type: String) {
            self.id = id
            self.type = type
        }
    }

    public let challenge: String
    public let timeout: Int
    public let rpId: String
    public let allowCredentials: [CredentialDescriptor]
    public let userVerification: String?

    public init(
        challenge: String,
        timeout: Int,
        rpId: String,
        allowCredentials: [CredentialDescriptor],
        userVerification: String? = nil
    ) {
        self.challenge = challenge
        self.timeout = timeout
        self.rpId = rpId
        self.allowCredentials = allowCredentials
        self.userVerification = userVerification
    }

    init(_ options: WebAuthnLoginOptions) {
        self.challenge = options.challenge
        self.timeout = options.timeout
        self.rpId = options.rpId
        self.allowCredentials = options.allowCredentials.map {
            CredentialDescriptor(id: $0.id, type: $0.type)
        }
        self.userVerification = (options.rawValue["userVerification"] as? String)
            ?? (options.rawValue["user_verification"] as? String)
    }
}

public struct PasskeyAssertionResult: Sendable {
    public struct Response: Sendable {
        public let clientDataJSON: String
        public let authenticatorData: String
        public let signature: String
        public let userHandle: String?

        public init(
            clientDataJSON: String,
            authenticatorData: String,
            signature: String,
            userHandle: String?
        ) {
            self.clientDataJSON = clientDataJSON
            self.authenticatorData = authenticatorData
            self.signature = signature
            self.userHandle = userHandle
        }
    }

    public let id: String
    public let rawId: String
    public let type: String
    public let response: Response

    public init(
        id: String,
        rawId: String,
        type: String = "public-key",
        response: Response
    ) {
        self.id = id
        self.rawId = rawId
        self.type = type
        self.response = response
    }

    public func toDictionary() -> [String: Any] {
        let userHandle: Any = response.userHandle ?? NSNull()
        return [
            "id": id,
            "rawId": rawId,
            "type": type,
            "response": [
                "clientDataJSON": response.clientDataJSON,
                "authenticatorData": response.authenticatorData,
                "signature": response.signature,
                "userHandle": userHandle,
            ],
            "clientExtensionResults": [:],
        ]
    }
}

public protocol PasskeyAuthenticating {
    func createAssertionResponse(options: PasskeyAssertionRequestOptions) async throws -> PasskeyAssertionResult
}

public struct PasskeyAttestationCreationOptions: Sendable {
    public let challenge: String
    public let rpId: String?
    public let rpName: String
    public let userId: String
    public let userName: String
    public let userDisplayName: String
    public let pubKeyCredParams: [WebAuthnPubKeyCredParam]
    public let timeout: Int
    public let attestation: String?

    init(_ options: WebAuthnEnrollBeginResponse) {
        self.challenge = options.challenge
        self.rpId = options.rp.id
        self.rpName = options.rp.name
        self.userId = options.user.id
        self.userName = options.user.name
        self.userDisplayName = options.user.displayName
        self.pubKeyCredParams = options.pubKeyCredParams
        self.timeout = options.timeout
        self.attestation = options.attestation
    }
}

public typealias PasskeyAttestationResult = WebAuthnAttestationResponse

public protocol PasskeyAttestationAuthenticating {
    func createAttestationResponse(options: PasskeyAttestationCreationOptions) async throws -> PasskeyAttestationResult
}

public extension AuthClient {
    func signInWithPasskey(
        email: String,
        authenticator: any PasskeyAuthenticating
    ) async throws -> AuthResponse {
        let begin = try await beginWebAuthnLogin(email: email)
        let options = PasskeyAssertionRequestOptions(begin.options)
        let assertionResponse = try await authenticator.createAssertionResponse(options: options)
        return try await finishWebAuthnLogin(
            challengeId: begin.challengeId,
            assertionResponse: assertionResponse.toDictionary()
        )
    }

    func enrollPasskey(
        displayName: String = "",
        authenticator: any PasskeyAttestationAuthenticating
    ) async throws -> WebAuthnEnrollConfirmResponse {
        let begin = try await enrollWebAuthn()
        let attestationResponse = try await authenticator.createAttestationResponse(
            options: PasskeyAttestationCreationOptions(begin)
        )
        return try await confirmWebAuthnEnrollment(
            WebAuthnEnrollConfirmRequest(
                displayName: displayName,
                attestationResponse: attestationResponse
            )
        )
    }

    func verifyPasskey(
        mfaToken: String,
        authenticator: any PasskeyAuthenticating
    ) async throws -> AuthResponse {
        let challenge = try await webauthnChallenge(mfaToken: mfaToken)
        let assertionResponse = try await authenticator.createAssertionResponse(
            options: PasskeyAssertionRequestOptions(challenge.options)
        )
        return try await webauthnVerify(
            mfaToken: mfaToken,
            challengeId: challenge.challengeId,
            assertionResponse: assertionResponse.toDictionary()
        )
    }
}

#if canImport(AuthenticationServices)
import AuthenticationServices

public extension AuthClient {
    func signInWithPasskey(email: String) async throws -> AuthResponse {
        try await signInWithPasskey(
            email: email,
            authenticator: SystemPasskeyAuthenticator()
        )
    }
}

public enum PasskeyAuthenticationError: Error, LocalizedError {
    case invalidBase64URL(field: String)
    case unexpectedCredentialType(String)

    public var errorDescription: String? {
        switch self {
        case .invalidBase64URL(let field):
            return "Invalid base64url value for \(field)"
        case .unexpectedCredentialType(let typeName):
            return "Unexpected passkey credential type: \(typeName)"
        }
    }
}

public final class SystemPasskeyAuthenticator: PasskeyAuthenticating, @unchecked Sendable {
    private let presentationContextProvider: (any ASAuthorizationControllerPresentationContextProviding)?
    private let performer: any AuthorizationAssertionPerforming

    public init(
        presentationContextProvider: (any ASAuthorizationControllerPresentationContextProviding)? = nil
    ) {
        self.presentationContextProvider = presentationContextProvider
        self.performer = AuthorizationControllerAssertionPerformer()
    }

    public func createAssertionResponse(
        options: PasskeyAssertionRequestOptions
    ) async throws -> PasskeyAssertionResult {
        try await runAssertionRequest(options: options)
    }

    @MainActor
    func makeAssertionRequest(
        options: PasskeyAssertionRequestOptions
    ) throws -> ASAuthorizationPlatformPublicKeyCredentialAssertionRequest {
        let challenge = try decodeBase64URL(
            options.challenge,
            field: "PasskeyAssertionRequestOptions.challenge"
        )
        let provider = ASAuthorizationPlatformPublicKeyCredentialProvider(
            relyingPartyIdentifier: options.rpId
        )
        let request = provider.createCredentialAssertionRequest(challenge: challenge)
        request.allowedCredentials = try options.allowCredentials.map { credential in
            ASAuthorizationPlatformPublicKeyCredentialDescriptor(
                credentialID: try decodeBase64URL(
                    credential.id,
                    field: "PasskeyAssertionRequestOptions.allowCredentials.id"
                )
            )
        }
        request.userVerificationPreference = userVerificationPreference(for: options.userVerification)
        return request
    }

    func serializeAssertion(
        _ assertion: any ASAuthorizationPublicKeyCredentialAssertion
    ) -> PasskeyAssertionResult {
        serializePasskeyAssertion(assertion)
    }

    private func userVerificationPreference(
        for requested: String?
    ) -> ASAuthorizationPublicKeyCredentialUserVerificationPreference {
        switch requested {
        case "required":
            return .required
        case "discouraged":
            return .discouraged
        default:
            return .preferred
        }
    }

    @MainActor
    private func runAssertionRequest(
        options: PasskeyAssertionRequestOptions
    ) async throws -> PasskeyAssertionResult {
        let request = try makeAssertionRequest(options: options)
        return try await performer.perform(
            request: request,
            presentationContextProvider: presentationContextProvider
        )
    }
}

@MainActor
private protocol AuthorizationAssertionPerforming {
    func perform(
        request: ASAuthorizationPlatformPublicKeyCredentialAssertionRequest,
        presentationContextProvider: (any ASAuthorizationControllerPresentationContextProviding)?
    ) async throws -> PasskeyAssertionResult
}

@MainActor
private struct AuthorizationControllerAssertionPerformer: AuthorizationAssertionPerforming {
    func perform(
        request: ASAuthorizationPlatformPublicKeyCredentialAssertionRequest,
        presentationContextProvider: (any ASAuthorizationControllerPresentationContextProviding)?
    ) async throws -> PasskeyAssertionResult {
        let operation = AuthorizationControllerAssertionOperation(
            presentationContextProvider: presentationContextProvider
        )
        return try await operation.perform(request: request)
    }
}

@MainActor
private final class AuthorizationControllerAssertionOperation: NSObject, ASAuthorizationControllerDelegate {
    private let presentationContextProvider: (any ASAuthorizationControllerPresentationContextProviding)?
    private var controller: ASAuthorizationController?
    private var continuation: CheckedContinuation<PasskeyAssertionResult, Error>?

    init(
        presentationContextProvider: (any ASAuthorizationControllerPresentationContextProviding)?
    ) {
        self.presentationContextProvider = presentationContextProvider
    }

    func perform(
        request: ASAuthorizationPlatformPublicKeyCredentialAssertionRequest
    ) async throws -> PasskeyAssertionResult {
        let controller = ASAuthorizationController(authorizationRequests: [request])
        self.controller = controller
        controller.delegate = self
        controller.presentationContextProvider = presentationContextProvider
        return try await withCheckedThrowingContinuation { continuation in
            self.continuation = continuation
            controller.performRequests()
        }
    }

    func authorizationController(
        controller: ASAuthorizationController,
        didCompleteWithAuthorization authorization: ASAuthorization
    ) {
        guard let assertion = authorization.credential as? ASAuthorizationPlatformPublicKeyCredentialAssertion else {
            finish(with: .failure(
                PasskeyAuthenticationError.unexpectedCredentialType(
                    String(describing: type(of: authorization.credential))
                )
            ))
            return
        }
        finish(with: .success(serializePasskeyAssertion(assertion)))
    }

    func authorizationController(
        controller: ASAuthorizationController,
        didCompleteWithError error: any Error
    ) {
        finish(with: .failure(error))
    }

    private func finish(
        with result: Result<PasskeyAssertionResult, Error>
    ) {
        guard let continuation else {
            return
        }
        self.continuation = nil
        self.controller = nil
        switch result {
        case .success(let assertion):
            continuation.resume(returning: assertion)
        case .failure(let error):
            continuation.resume(throwing: error)
        }
    }
}

private func decodeBase64URL(_ value: String, field: String) throws -> Data {
    let normalized = normalizedBase64URL(value)
    guard let data = Data(base64Encoded: normalized) else {
        throw PasskeyAuthenticationError.invalidBase64URL(field: field)
    }
    return data
}

private func serializePasskeyAssertion(
    _ assertion: any ASAuthorizationPublicKeyCredentialAssertion
) -> PasskeyAssertionResult {
    PasskeyAssertionResult(
        id: encodeBase64URL(assertion.credentialID),
        rawId: encodeBase64URL(assertion.credentialID),
        response: .init(
            clientDataJSON: encodeBase64URL(assertion.rawClientDataJSON),
            authenticatorData: encodeBase64URL(assertion.rawAuthenticatorData),
            signature: encodeBase64URL(assertion.signature),
            userHandle: assertion.userID.isEmpty ? nil : encodeBase64URL(assertion.userID)
        )
    )
}

private func encodeBase64URL(_ data: Data) -> String {
    data.base64EncodedString()
        .replacingOccurrences(of: "+", with: "-")
        .replacingOccurrences(of: "/", with: "_")
        .replacingOccurrences(of: "=", with: "")
}

private func normalizedBase64URL(_ value: String) -> String {
    let base64 = value
        .replacingOccurrences(of: "-", with: "+")
        .replacingOccurrences(of: "_", with: "/")
    let paddingCount = (4 - (base64.count % 4)) % 4
    return base64 + String(repeating: "=", count: paddingCount)
}
#endif
