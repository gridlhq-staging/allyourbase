import Foundation
import Testing
@testable import Allyourbase

struct ContractFixtureTests {
    @Test func authResponseFixtureDecodesFromCanonicalShapes() throws {
        let response = try AuthResponse.decode(ContractFixtures.authResponse)

        #expect(response.token == "jwt_stage3")
        #expect(response.refreshToken == "refresh_stage3")
        #expect(response.user.id == "usr_1")
        #expect(response.user.email == "dev@allyourbase.io")
        #expect(response.user.emailVerified == true)
        #expect(response.user.createdAt == "2026-01-01T00:00:00Z")
        #expect(response.user.updatedAt == nil)
    }

    @Test func magicLinkFixturesDecodeThroughModelOwners() throws {
        let request = try MagicLinkRequestResponse.decode(ContractFixtures.magicLinkRequestResponse)
        #expect(request.message == "If an account exists, a magic link has been sent.")

        let confirm = try MagicLinkConfirmResponse.decode(ContractFixtures.magicLinkConfirmResponse)
        switch confirm {
        case .authenticated(let auth):
            #expect(auth.user.email == "magic@allyourbase.io")
            #expect(auth.user.emailVerified == true)
            #expect(auth.user.createdAt == "2026-05-01T12:00:00Z")
            #expect(auth.user.updatedAt == nil)
        case .pendingMFA:
            Issue.record("expected authenticated fixture")
        }
    }

    @Test func pendingMFAMagicLinkFixtureDecodesThroughModelOwner() throws {
        let response = try MagicLinkConfirmResponse.decode(ContractFixtures.magicLinkConfirmPendingMFAResponse)
        switch response {
        case .pendingMFA(let mfaToken):
            #expect(mfaToken == "mfa_pending_token_stage1")
        case .authenticated:
            Issue.record("expected pending MFA fixture")
        }
    }

    @Test func webAuthnDiscoverBeginFixtureDecodesWithoutAllowCredentials() throws {
        let response = try WebAuthnLoginBeginResponse.decode(ContractFixtures.webAuthnDiscoverBeginResponse)

        #expect(response.challengeId == "webauthn_discover_challenge_fixture")
        #expect(response.options.challenge == "webauthn_discover_challenge")
        #expect(response.options.rpId == "127.0.0.1")
        #expect(response.options.timeout == 300000)
        #expect(response.options.allowCredentials.isEmpty == true)
    }

    @Test func webAuthnEnrollBeginFixtureDecodesThroughModelOwner() throws {
        let response = try WebAuthnEnrollBeginResponse.decode(ContractFixtures.webAuthnEnrollBeginResponse)

        #expect(response.challenge == "webauthn_enroll_begin_challenge")
        #expect(response.rp.id == "127.0.0.1")
        #expect(response.rp.name == "Allyourbase")
        #expect(response.user.id == "webauthn_enroll_user_id")
        #expect(response.user.name == "webauthn-e2e@example.com")
        #expect(response.user.displayName == "webauthn-e2e@example.com")
        #expect(response.timeout == 300000)
        #expect(response.attestation == "none")
        let algorithms = response.pubKeyCredParams.map { parameter in parameter.alg }
        let credentialTypes = response.pubKeyCredParams.map { parameter in parameter.type }
        #expect(algorithms == [-7, -35, -36, -257, -258, -259, -37, -38, -39, -8])
        #expect(credentialTypes.allSatisfy { $0 == "public-key" })
    }

    @Test func webAuthnEnrollConfirmFixturesDecodeThroughModelOwners() throws {
        let request = try WebAuthnEnrollConfirmRequest(from: ContractFixtures.webAuthnEnrollConfirmRequest)

        #expect(request.displayName == "Primary security key")
        #expect(request.attestationResponse.id == "webauthn_enroll_credential")
        #expect(request.attestationResponse.rawId == "webauthn_enroll_credential")
        #expect(request.attestationResponse.type == "public-key")
        #expect(request.attestationResponse.response.attestationObject == "webauthn_enroll_attestation_object")
        #expect(request.attestationResponse.response.clientDataJSON == "eyJ0eXBlIjoid2ViYXV0aG4uY3JlYXRlIiwiY2hhbGxlbmdlIjoid2ViYXV0aG5fZW5yb2xsX2JlZ2luX2NoYWxsZW5nZSIsIm9yaWdpbiI6Imh0dHA6Ly8xMjcuMC4wLjE6ODA5MCJ9")
        #expect(request.attestationResponse.response.transports == ["internal"])

        let response = try WebAuthnEnrollConfirmResponse.decode(ContractFixtures.webAuthnEnrollConfirmResponse)
        #expect(response.message == "WebAuthn MFA enrollment confirmed")
    }

    @Test func webAuthnMFAVerifyRequestFixtureRoundTripsCanonicalWireJSON() throws {
        let request = try WebAuthnMFAVerifyRequest(from: ContractFixtures.webAuthnMFAVerifyRequest)

        #expect(request.challengeId == "webauthn_mfa_challenge_fixture")
        #expect(request.assertionResponse.id == "webauthn_mfa_credential")
        #expect(request.assertionResponse.response.authenticatorData == "EsoXtJryKJQ28wPgFmAwoh5SXSZuIJJnQzgBqP1AcaAFAAAAAQ")
        #expect(request.assertionResponse.response.signature == "webauthn_mfa_signature")
        #expect(request.assertionResponse.response.userHandle == "webauthn_mfa_user_handle")

        let data = try JSONSerialization.data(withJSONObject: request.toDictionary(), options: [.sortedKeys])
        let serialized = try #require(JSONSerialization.jsonObject(with: data) as? [String: Any])
        try assertJSONDictionariesEqual(serialized, ContractFixtures.webAuthnMFAVerifyRequest)

        #expect(serialized["challenge_id"] as? String == "webauthn_mfa_challenge_fixture")
        #expect(serialized["challengeId"] == nil)
        #expect(serialized["assertion_response"] != nil)
        #expect(serialized["assertionResponse"] == nil)
    }

    @Test func webAuthnMFAVerifyResponseFixtureDecodesThroughAuthResponseOwner() throws {
        let response = try AuthResponse.decode(ContractFixtures.webAuthnMFAVerifyResponse)

        #expect(response.token == "jwt_webauthn_mfa")
        #expect(response.refreshToken == "refresh_webauthn_mfa")
        #expect(response.user.id == "usr_webauthn_mfa")
        #expect(response.user.email == "webauthn-e2e@example.com")
        #expect(response.user.createdAt == "2026-07-11T00:00:00Z")
        #expect(response.user.updatedAt == "2026-07-11T00:00:00Z")
    }

    @Test func linkEmailFixtureNormalizesUserAliasFields() throws {
        let response = try AuthResponse.decode(ContractFixtures.linkEmailResponse)
        #expect(response.user.linkedAt != nil)
        #expect(response.user.emailVerified == nil)
        #expect(response.user.createdAt != nil)
        #expect(response.user.updatedAt != nil)
    }

    @Test func listFixtureMetadataAndItemsPreserveOrder() throws {
        let response = try ListResponse.decode(ContractFixtures.listResponse) { item in
            item
        }

        #expect(response.metadata.totalItems == 2)
        let firstItem = try #require(response.items.first)
        #expect(firstItem["id"] as? String == "rec_1")
        let lastItem = try #require(response.items.last)
        #expect(lastItem["id"] as? String == "rec_2")
    }

    @Test func optionalHelpersFallThroughOnTypeMismatch() throws {
        // If "createdAt" exists with the wrong type, "created_at" should still be found
        let json: [String: Any] = [
            "id": "usr_2",
            "email": "test@example.com",
            "createdAt": 12345,
            "created_at": "2026-02-02T00:00:00Z",
            "emailVerified": "yes",
            "email_verified": true,
        ]
        let user = try User(from: json)
        #expect(user.createdAt == "2026-02-02T00:00:00Z")
        #expect(user.emailVerified == true)
    }

    @Test func recordFixtureAcceptsSnakeCaseMapping() throws {
        let fixture = ContractFixtures.recordPayload

        #expect(fixture["author_id"] as? Int == 1)

        let response: [String: Any] = [
            "items": [fixture],
            "page": 1,
            "perPage": 1,
            "totalItems": 1,
            "totalPages": 1,
        ]

        let list = try ListResponse.decode(response) { item in
            item
        }
        #expect(list.items[0]["created_at"] as? String == "2026-01-01T00:00:00Z")
    }

    @Test func storageObjectFixtureDecodesFromCanonicalShape() throws {
        let object = try StorageObject(from: ContractFixtures.storageObject)
        #expect(object.id == "file_abc123")
        #expect(object.bucket == "uploads")
        #expect(object.name == "document.pdf")
    }

    @Test func storageListFixtureDecodesNullVariantsFromCanonicalShape() throws {
        let response = try StorageListResponse(from: ContractFixtures.storageListResponse)
        #expect(response.totalItems == 2)
        #expect(response.items.count == 2)
        #expect(response.items[0].userId == "usr_1")
        #expect(response.items[0].updatedAt == nil)
        #expect(response.items[1].userId == nil)
        #expect(response.items[1].updatedAt == nil)
    }

    @Test func errorFixtureDecodesNumericCodeFromCanonicalShape() throws {
        let error = try decodeError(ContractFixtures.errorWithNumericCode, status: 403)
        #expect(error.status == 403)
        #expect(error.message == "forbidden")
        #expect(error.code == "403")
        #expect(error.docUrl == "https://allyourbase.io/docs/errors#forbidden")
        #expect(error.data?["resource"] as? String == "posts")
    }

    @Test func errorFixtureDecodesStringCodeFromCanonicalShape() throws {
        let error = try decodeError(ContractFixtures.errorWithStringCode)
        #expect(error.status == 400)
        #expect(error.message == "Missing refresh token")
        #expect(error.code == "auth/missing-refresh-token")
        #expect(error.data?["detail"] as? String == "refresh token not available")
    }

    @Test func realtimeEventFixtureDecodesFromCanonicalShape() throws {
        let event = try RealtimeEvent(from: ContractFixtures.realtimeEvent)
        #expect(event.action == "UPDATE")
        #expect(event.table == "posts")
        #expect(event.record["id"] as? String == "rec_1")
        let oldRecord = try #require(event.oldRecord)
        #expect(oldRecord["title"] as? String == "before")
    }

    private func decodeError(
        _ payload: [String: Any],
        status: Int = 400
    ) throws -> AYBError {
        let body = try JSONSerialization.data(withJSONObject: payload)
        let response = HTTPResponse(
            statusCode: status,
            statusText: "bad request",
            headers: [:],
            body: body
        )
        return AYBError.from(response: response)
    }

    private func assertJSONDictionariesEqual(
        _ actual: [String: Any],
        _ expected: [String: Any]
    ) throws {
        let actualData = try JSONSerialization.data(withJSONObject: actual, options: [.sortedKeys])
        let expectedData = try JSONSerialization.data(withJSONObject: expected, options: [.sortedKeys])
        #expect(String(data: actualData, encoding: .utf8) == String(data: expectedData, encoding: .utf8))
    }
}
