import Foundation

enum ContractFixtures {
    private static let fixtureRoot = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .appendingPathComponent("tests/contract/fixtures")

    private static func loadFixture(at relativePath: String) -> [String: Any] {
        let url = fixtureRoot.appendingPathComponent(relativePath)
        guard let data = try? Data(contentsOf: url),
              let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            fatalError("Failed to load fixture: \(relativePath)")
        }
        return json
    }

    private static func loadArrayFixture(at relativePath: String) -> [[String: Any]] {
        let url = fixtureRoot.appendingPathComponent(relativePath)
        guard let data = try? Data(contentsOf: url),
              let json = try? JSONSerialization.jsonObject(with: data) as? [[String: Any]] else {
            fatalError("Failed to load fixture array: \(relativePath)")
        }
        return json
    }

    private static func loadParityResponse(_ name: String) -> [String: Any] {
        let fixture = loadFixture(at: "sdk_parity/\(name)")
        guard let response = fixture["response"] as? [String: Any] else {
            fatalError("Missing response object in parity fixture: \(name)")
        }
        return response
    }

    nonisolated(unsafe) static let authResponse: [String: Any] = [
        "token": "jwt_stage3",
        "refreshToken": "refresh_stage3",
        "user": [
            "id": "usr_1",
            "email": "dev@allyourbase.io",
            "email_verified": true,
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": NSNull(),
        ],
    ]

    nonisolated(unsafe) static let anonymousAuthResponse: [String: Any] = loadParityResponse("anonymous.json")
    nonisolated(unsafe) static let magicLinkRequestResponse: [String: Any] = loadFixture(at: "sdk_contract/magic_link_request_response.json")
    nonisolated(unsafe) static let magicLinkConfirmResponse: [String: Any] = loadFixture(at: "sdk_contract/magic_link_confirm_success_response.json")
    nonisolated(unsafe) static let magicLinkConfirmPendingMFAResponse: [String: Any] = loadFixture(at: "sdk_contract/magic_link_confirm_pending_mfa_response.json")
    nonisolated(unsafe) static let webAuthnLoginBeginResponse: [String: Any] = loadFixture(at: "sdk_contract/webauthn_login_begin_response.json")
    nonisolated(unsafe) static let webAuthnDiscoverBeginResponse: [String: Any] = loadFixture(at: "sdk_contract/webauthn_discover_begin_response.json")
    nonisolated(unsafe) static let webAuthnDiscoverFinishRequest: [String: Any] = loadFixture(at: "sdk_contract/webauthn_discover_finish_request.json")
    nonisolated(unsafe) static let oauthStartURLCases: [[String: Any]] = loadArrayFixture(at: "sdk_contract/oauth_start_url_cases.json")
    nonisolated(unsafe) static let webAuthnEnrollBeginResponse: [String: Any] = loadFixture(at: "sdk_contract/webauthn_enroll_begin_response.json")
    nonisolated(unsafe) static let webAuthnEnrollConfirmRequest: [String: Any] = loadFixture(at: "sdk_contract/webauthn_enroll_confirm_request.json")
    nonisolated(unsafe) static let webAuthnEnrollConfirmResponse: [String: Any] = loadFixture(at: "sdk_contract/webauthn_enroll_confirm_response.json")
    nonisolated(unsafe) static let webAuthnMFAChallengeResponse: [String: Any] = loadFixture(at: "sdk_contract/webauthn_mfa_challenge_response.json")
    nonisolated(unsafe) static let webAuthnMFAVerifyRequest: [String: Any] = loadFixture(at: "sdk_contract/webauthn_mfa_verify_request.json")
    nonisolated(unsafe) static let webAuthnMFAVerifyResponse: [String: Any] = loadFixture(at: "sdk_contract/webauthn_mfa_verify_response.json")
    nonisolated(unsafe) static let searchSynonymsResponse: [String: Any] = loadFixture(at: "sdk_contract/search_synonyms_response.json")
    nonisolated(unsafe) static let searchSynonymsRequest: [String: Any] = loadFixture(at: "sdk_contract/search_synonyms_request.json")
    nonisolated(unsafe) static let linkEmailResponse: [String: Any] = loadParityResponse("link_email.json")

    nonisolated(unsafe) static let recordPayload: [String: Any] = [
        "id": "rec_1",
        "title": "Hello",
        "author_id": 1,
        "created_at": "2026-01-01T00:00:00Z",
        "updated_at": NSNull(),
    ]

    nonisolated(unsafe) static let listResponse: [String: Any] = [
        "items": [
            [
                "id": "rec_1",
                "title": "First",
            ],
            [
                "id": "rec_2",
                "title": "Second",
            ],
        ],
        "page": 1,
        "perPage": 2,
        "totalItems": 2,
        "totalPages": 1,
    ]

    nonisolated(unsafe) static let errorWithNumericCode: [String: Any] = [
        "code": 403,
        "message": "forbidden",
        "data": [
            "resource": "posts"
        ],
        "doc_url": "https://allyourbase.io/docs/errors#forbidden",
    ]

    nonisolated(unsafe) static let storageObject: [String: Any] = [
        "id": "file_abc123",
        "bucket": "uploads",
        "name": "document.pdf",
        "size": 1024,
        "contentType": "application/pdf",
        "userId": "usr_1",
        "createdAt": "2026-01-01T00:00:00Z",
        "updatedAt": "2026-01-02T12:30:00Z",
    ]

    nonisolated(unsafe) static let storageListResponse: [String: Any] = [
        "items": [
            [
                "id": "file_1",
                "bucket": "uploads",
                "name": "doc1.pdf",
                "size": 1024,
                "contentType": "application/pdf",
                "userId": "usr_1",
                "createdAt": "2026-01-01T00:00:00Z",
                "updatedAt": NSNull(),
            ],
            [
                "id": "file_2",
                "bucket": "uploads",
                "name": "image.png",
                "size": 2048,
                "contentType": "image/png",
                "userId": NSNull(),
                "createdAt": "2026-01-02T00:00:00Z",
                "updatedAt": NSNull(),
            ],
        ],
        "totalItems": 2,
    ]

    nonisolated(unsafe) static let errorWithStringCode: [String: Any] = [
        "code": "auth/missing-refresh-token",
        "message": "Missing refresh token",
        "data": [
            "detail": "refresh token not available"
        ],
    ]

    nonisolated(unsafe) static let realtimeEvent: [String: Any] = [
        "action": "UPDATE",
        "table": "posts",
        "record": [
            "id": "rec_1",
            "title": "after",
        ],
        "oldRecord": [
            "id": "rec_1",
            "title": "before",
        ],
    ]
}
