import Foundation

public struct WebAuthnRelyingParty {
    public let id: String?
    public let name: String

    public init(id: String? = nil, name: String) {
        self.id = id
        self.name = name
    }

    public init(from json: [String: Any]) throws {
        self.id = AYBJSON.optionalString(json, ["id"])
        self.name = try AYBJSON.requiredString(json, ["name"], "WebAuthnRelyingParty.name")
    }
}

public struct WebAuthnUserEntity {
    public let id: String
    public let name: String
    public let displayName: String

    public init(id: String, name: String, displayName: String) {
        self.id = id
        self.name = name
        self.displayName = displayName
    }

    public init(from json: [String: Any]) throws {
        self.id = try AYBJSON.requiredString(json, ["id"], "WebAuthnUserEntity.id")
        self.name = try AYBJSON.requiredString(json, ["name"], "WebAuthnUserEntity.name")
        self.displayName = try AYBJSON.requiredString(
            json,
            ["displayName", "display_name"],
            "WebAuthnUserEntity.displayName"
        )
    }
}

public struct WebAuthnPubKeyCredParam: Sendable {
    public let type: String
    public let alg: Int

    public init(type: String, alg: Int) {
        self.type = type
        self.alg = alg
    }

    public init(from json: [String: Any]) throws {
        self.type = try AYBJSON.requiredString(json, ["type"], "WebAuthnPubKeyCredParam.type")
        self.alg = try AYBJSON.requiredInt(json, ["alg"])
    }
}

public struct WebAuthnEnrollBeginResponse {
    public let rawValue: [String: Any]
    public let challenge: String
    public let rp: WebAuthnRelyingParty
    public let user: WebAuthnUserEntity
    public let pubKeyCredParams: [WebAuthnPubKeyCredParam]
    public let timeout: Int
    public let attestation: String?

    public init(from json: [String: Any]) throws {
        self.rawValue = json
        self.challenge = try AYBJSON.requiredString(json, ["challenge"], "WebAuthnEnrollBeginResponse.challenge")
        self.rp = try WebAuthnRelyingParty(from: try AYBJSON.requiredDictionary(json, "rp"))
        self.user = try WebAuthnUserEntity(from: try AYBJSON.requiredDictionary(json, "user"))
        self.pubKeyCredParams = try decodeWebAuthnArray(
            json["pubKeyCredParams"],
            "WebAuthnEnrollBeginResponse.pubKeyCredParams"
        ) { try WebAuthnPubKeyCredParam(from: $0) }
        self.timeout = try AYBJSON.requiredInt(json, ["timeout"])
        self.attestation = AYBJSON.optionalString(json, ["attestation"])
    }

    public static func decode(_ json: Any) throws -> WebAuthnEnrollBeginResponse {
        try WebAuthnEnrollBeginResponse(from: try AYBJSON.expectDictionary(json, "WebAuthnEnrollBeginResponse"))
    }
}

public struct WebAuthnAttestationResponse {
    public struct Response {
        public let attestationObject: String
        public let clientDataJSON: String
        public let transports: [String]

        public init(attestationObject: String, clientDataJSON: String, transports: [String] = []) {
            self.attestationObject = attestationObject
            self.clientDataJSON = clientDataJSON
            self.transports = transports
        }
    }

    public let id: String
    public let rawId: String
    public let type: String
    public let response: Response
    private let clientExtensionResults: [String: Any]

    public init(id: String, rawId: String, type: String = "public-key", response: Response) {
        self.id = id
        self.rawId = rawId
        self.type = type
        self.response = response
        self.clientExtensionResults = [:]
    }

    init(fromDictionary json: [String: Any]) {
        self.id = json["id"] as? String ?? ""
        self.rawId = json["rawId"] as? String ?? ""
        self.type = json["type"] as? String ?? "public-key"
        let response = json["response"] as? [String: Any] ?? [:]
        self.response = Response(
            attestationObject: response["attestationObject"] as? String ?? "",
            clientDataJSON: response["clientDataJSON"] as? String ?? "",
            transports: response["transports"] as? [String] ?? []
        )
        self.clientExtensionResults = json["clientExtensionResults"] as? [String: Any] ?? [:]
    }

    public init(from json: [String: Any]) throws {
        self.init(fromDictionary: json)
    }

    public func toDictionary() -> [String: Any] {
        [
            "id": id,
            "rawId": rawId,
            "type": type,
            "response": [
                "attestationObject": response.attestationObject,
                "clientDataJSON": response.clientDataJSON,
                "transports": response.transports,
            ],
            "clientExtensionResults": clientExtensionResults,
        ]
    }
}

public struct WebAuthnEnrollConfirmRequest {
    public let displayName: String
    public let attestationResponse: WebAuthnAttestationResponse

    public init(displayName: String, attestationResponse: WebAuthnAttestationResponse) {
        self.displayName = displayName
        self.attestationResponse = attestationResponse
    }

    public init(from json: [String: Any]) throws {
        self.displayName = try AYBJSON.requiredString(
            json,
            ["display_name", "displayName"],
            "WebAuthnEnrollConfirmRequest.displayName"
        )
        self.attestationResponse = try WebAuthnAttestationResponse(
            from: try AYBJSON.requiredDictionary(json, "attestation_response")
        )
    }

    public func toDictionary() -> [String: Any] {
        ["display_name": displayName, "attestation_response": attestationResponse.toDictionary()]
    }
}

public struct WebAuthnEnrollConfirmResponse {
    public let message: String

    public init(message: String) {
        self.message = message
    }

    public init(from json: [String: Any]) throws {
        self.message = try AYBJSON.requiredString(json, ["message"], "WebAuthnEnrollConfirmResponse.message")
    }

    public static func decode(_ json: Any) throws -> WebAuthnEnrollConfirmResponse {
        try WebAuthnEnrollConfirmResponse(from: try AYBJSON.expectDictionary(json, "WebAuthnEnrollConfirmResponse"))
    }
}

public struct WebAuthnMFAChallengeResponse {
    public let challengeId: String
    public let options: WebAuthnLoginOptions

    public init(from json: [String: Any]) throws {
        self.challengeId = try AYBJSON.requiredString(
            json,
            ["challenge_id", "challengeId"],
            "WebAuthnMFAChallengeResponse.challengeId"
        )
        self.options = try WebAuthnLoginOptions(from: try AYBJSON.requiredDictionary(json, "options"))
    }

    public static func decode(_ json: Any) throws -> WebAuthnMFAChallengeResponse {
        try WebAuthnMFAChallengeResponse(from: try AYBJSON.expectDictionary(json, "WebAuthnMFAChallengeResponse"))
    }
}

public struct WebAuthnAssertionResponse {
    public struct Response {
        public let clientDataJSON: String
        public let authenticatorData: String
        public let signature: String
        public let userHandle: String?
    }

    public let id: String
    public let rawId: String
    public let type: String
    public let response: Response
    private let clientExtensionResults: [String: Any]

    init(fromDictionary json: [String: Any]) {
        self.id = json["id"] as? String ?? ""
        self.rawId = json["rawId"] as? String ?? ""
        self.type = json["type"] as? String ?? "public-key"
        let response = json["response"] as? [String: Any] ?? [:]
        self.response = Response(
            clientDataJSON: response["clientDataJSON"] as? String ?? "",
            authenticatorData: response["authenticatorData"] as? String ?? "",
            signature: response["signature"] as? String ?? "",
            userHandle: response["userHandle"] as? String
        )
        self.clientExtensionResults = json["clientExtensionResults"] as? [String: Any] ?? [:]
    }

    public init(from json: [String: Any]) throws {
        self.init(fromDictionary: json)
    }

    public func toDictionary() -> [String: Any] {
        var responseDict: [String: Any] = [
            "clientDataJSON": response.clientDataJSON,
            "authenticatorData": response.authenticatorData,
            "signature": response.signature,
        ]
        responseDict["userHandle"] = response.userHandle ?? NSNull()
        return [
            "id": id,
            "rawId": rawId,
            "type": type,
            "response": responseDict,
            "clientExtensionResults": clientExtensionResults,
        ]
    }
}

public struct WebAuthnMFAVerifyRequest {
    public let challengeId: String
    public let assertionResponse: WebAuthnAssertionResponse

    public init(challengeId: String, assertionResponse: WebAuthnAssertionResponse) {
        self.challengeId = challengeId
        self.assertionResponse = assertionResponse
    }

    public init(from json: [String: Any]) throws {
        self.challengeId = try AYBJSON.requiredString(
            json,
            ["challenge_id", "challengeId"],
            "WebAuthnMFAVerifyRequest.challengeId"
        )
        self.assertionResponse = try WebAuthnAssertionResponse(
            from: try AYBJSON.requiredDictionary(json, "assertion_response")
        )
    }

    public func toDictionary() -> [String: Any] {
        ["challenge_id": challengeId, "assertion_response": assertionResponse.toDictionary()]
    }
}

private func decodeWebAuthnArray<T>(
    _ value: Any?,
    _ context: String,
    decode: ([String: Any]) throws -> T
) throws -> [T] {
    try AYBJSON.expectArray(value, context).map { raw in
        try decode(try AYBJSON.expectDictionary(raw, context))
    }
}
