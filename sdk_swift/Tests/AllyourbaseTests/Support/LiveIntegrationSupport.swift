import Foundation
@testable import Allyourbase

enum LiveIntegrationSupportError: Error, CustomStringConvertible {
    case missingConfiguration(String)
    case timeout(String)
    case invalidCollectionIdentifier(String)
    case invalidLiveResponse(String)

    var description: String {
        switch self {
        case let .missingConfiguration(message):
            return message
        case let .timeout(message):
            return message
        case let .invalidCollectionIdentifier(message):
            return message
        case let .invalidLiveResponse(message):
            return message
        }
    }
}

enum RecordsLiveIntegrationSupport {
    static let collection = "sdk_swift_search_posts"
    static let waitIntervalNanoseconds: UInt64 = 250_000_000

    static func resolvedBaseURL(environment: [String: String] = ProcessInfo.processInfo.environment) -> String? {
        let rawValue = environment["AYB_TEST_URL"]?.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let rawValue, rawValue.isEmpty == false else {
            return nil
        }
        return rawValue.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
    }

    static func resolvedAdminToken(
        environment: [String: String] = ProcessInfo.processInfo.environment,
        readFile: (String) throws -> String = { path in
            try String(contentsOfFile: path, encoding: .utf8)
        }
    ) -> String? {
        let explicitToken =
            environment["AYB_TEST_ADMIN_TOKEN"]?.trimmingCharacters(in: .whitespacesAndNewlines) ??
            environment["AYB_ADMIN_TOKEN"]?.trimmingCharacters(in: .whitespacesAndNewlines)
        if let explicitToken, explicitToken.isEmpty == false {
            return explicitToken
        }

        let tokenPath: String?
        if let explicitPath = environment["AYB_ADMIN_TOKEN_PATH"]?.trimmingCharacters(in: .whitespacesAndNewlines),
           explicitPath.isEmpty == false {
            tokenPath = explicitPath
        } else if let homeDir = environment["HOME"]?.trimmingCharacters(in: .whitespacesAndNewlines),
                  homeDir.isEmpty == false {
            tokenPath = "\(homeDir)/.ayb/admin-token"
        } else {
            tokenPath = nil
        }

        guard let tokenPath else {
            return nil
        }

        do {
            let token = try readFile(tokenPath).trimmingCharacters(in: .whitespacesAndNewlines)
            return token.isEmpty ? nil : token
        } catch {
            return nil
        }
    }

    static func hasRequiredConfiguration(
        environment: [String: String] = ProcessInfo.processInfo.environment
    ) -> Bool {
        resolvedBaseURL(environment: environment) != nil &&
            resolvedAdminToken(environment: environment) != nil
    }

    static func newClient(
        environment: [String: String] = ProcessInfo.processInfo.environment
    ) throws -> AYBClient {
        guard let baseURL = resolvedBaseURL(environment: environment) else {
            throw LiveIntegrationSupportError.missingConfiguration(
                "Set AYB_TEST_URL before running live Swift SDK tests."
            )
        }

        guard let adminToken = resolvedAdminToken(environment: environment) else {
            throw LiveIntegrationSupportError.missingConfiguration(
                "Set AYB_TEST_ADMIN_TOKEN/AYB_ADMIN_TOKEN, or provide ~/.ayb/admin-token before running live Swift SDK tests."
            )
        }

        let client = AYBClient(baseURL)
        client.setApiKey(adminToken)
        return client
    }

    static func prepareSearchFixtures(using client: AYBClient, collection: String = collection) async throws {
        let safeCollection = try validatedCollectionIdentifier(collection)
        try await adminSQL(client, "DROP TABLE IF EXISTS \(safeCollection) CASCADE")
        try await adminSQL(
            client,
            """
            CREATE TABLE \(safeCollection) (
                id text PRIMARY KEY,
                title text NOT NULL,
                category text NOT NULL
            )
            """
        )
        try await adminSQL(client, "ALTER TABLE \(safeCollection) ENABLE ROW LEVEL SECURITY")
        try await adminSQL(
            client,
            "CREATE POLICY \(safeCollection)_all ON \(safeCollection) FOR ALL USING (true) WITH CHECK (true)"
        )
        try await adminSQL(
            client,
            """
            INSERT INTO \(safeCollection) (id, title, category) VALUES
                ('one', 'allyourbase migration guide', 'docs'),
                ('two', 'allyourbase search cookbook', 'docs'),
                ('three', 'postgres indexing handbook', 'guides')
            """
        )
        try await waitForCollection(client, collection: safeCollection)
    }

    static func dropSearchFixtures(using client: AYBClient, collection: String = collection) async throws {
        let safeCollection = try validatedCollectionIdentifier(collection)
        try await adminSQL(client, "DROP TABLE IF EXISTS \(safeCollection) CASCADE")
    }

    static func registerLiveUserClient(
        using adminClient: AYBClient,
        emailPrefix: String
    ) async throws -> (client: AYBClient, email: String, password: String) {
        guard let baseURL = resolvedBaseURL() else {
            throw LiveIntegrationSupportError.missingConfiguration(
                "Set AYB_TEST_URL before running live Swift SDK tests."
            )
        }
        let email = uniqueEmail(prefix: emailPrefix)
        let password = "SwiftLivePasskey123!"
        let client = AYBClient(baseURL)
        do {
            _ = try await client.auth.register(email: email, password: password)
            return (client, email, password)
        } catch {
            try? await deleteLiveUser(using: adminClient, email: email)
            throw error
        }
    }

    static func seedWebAuthnMFAFactor(using adminClient: AYBClient, email: String) async throws {
        try await adminSQL(
            adminClient,
            """
            INSERT INTO _ayb_user_mfa (
                user_id, method, phone, enabled, enrolled_at,
                webauthn_credential_id, webauthn_public_key,
                webauthn_sign_count, webauthn_display_name
            )
            SELECT id, 'webauthn', NULL, true, NOW(),
                   decode('c3dpZnQtbGl2ZS13ZWJhdXRobi1jcmVkZW50aWFs', 'base64'),
                   decode('c3dpZnQtbGl2ZS13ZWJhdXRobi1wdWJsaWMta2V5', 'base64'),
                   0, 'Swift live seeded passkey'
            FROM _ayb_users
            WHERE LOWER(email) = LOWER(\(sqlStringLiteral(email)))
            ON CONFLICT (user_id, method) DO UPDATE
            SET enabled = true,
                enrolled_at = NOW(),
                webauthn_credential_id = EXCLUDED.webauthn_credential_id,
                webauthn_public_key = EXCLUDED.webauthn_public_key,
                webauthn_sign_count = 0,
                webauthn_display_name = EXCLUDED.webauthn_display_name,
                webauthn_session_data = NULL
            """
        )
    }

    static func loginForMFAPendingToken(
        email: String,
        password: String,
        environment: [String: String] = ProcessInfo.processInfo.environment
    ) async throws -> String {
        guard let baseURL = resolvedBaseURL(environment: environment) else {
            throw LiveIntegrationSupportError.missingConfiguration(
                "Set AYB_TEST_URL before running live Swift SDK tests."
            )
        }
        let client = AYBClient(baseURL)
        let response: [String: Any] = try await client.request(
            "/api/auth/login",
            method: .post,
            body: ["email": email, "password": password],
            skipAuth: true,
            decode: { value in try AYBJSON.expectDictionary(value, "liveMFA.login") }
        )
        guard response["mfa_pending"] as? Bool == true,
              let token = response["mfa_token"] as? String,
              token.isEmpty == false else {
            throw LiveIntegrationSupportError.invalidLiveResponse(
                "Expected login to return a non-empty MFA-pending token."
            )
        }
        return token
    }

    static func deleteLiveUser(using adminClient: AYBClient, email: String) async throws {
        try await adminSQL(
            adminClient,
            "DELETE FROM _ayb_users WHERE LOWER(email) = LOWER(\(sqlStringLiteral(email)))"
        )
    }

    @discardableResult
    static func adminSQL(_ client: AYBClient, _ query: String) async throws -> [String: Any] {
        try await client.request(
            "/api/admin/sql",
            method: .post,
            body: ["query": query],
            decode: { value in
                try AYBJSON.expectDictionary(value, "adminSql")
            }
        ) as [String: Any]
    }

    private static func waitForCollection(_ client: AYBClient, collection: String) async throws {
        let deadline = Date().addingTimeInterval(30)
        while Date() < deadline {
            do {
                _ = try await client.records.list(collection)
                return
            } catch let error as AYBError {
                if error.status == 404 && error.message == "collection not found: \(collection)" {
                    try await Task.sleep(nanoseconds: waitIntervalNanoseconds)
                    continue
                }
                throw error
            }
        }

        throw LiveIntegrationSupportError.timeout(
            "Timed out waiting for \(collection) to become queryable."
        )
    }

    static func validatedCollectionIdentifier(_ collection: String) throws -> String {
        let normalized = collection.trimmingCharacters(in: .whitespacesAndNewlines)
        let allowed = CharacterSet.alphanumerics.union(CharacterSet(charactersIn: "_"))
        guard normalized.isEmpty == false,
              normalized.rangeOfCharacter(from: allowed.inverted) == nil else {
            throw LiveIntegrationSupportError.invalidCollectionIdentifier(
                "Collection identifiers must contain only letters, numbers, and underscores."
            )
        }
        return normalized
    }

    private static func uniqueEmail(prefix: String) -> String {
        let suffix = UUID().uuidString.lowercased().replacingOccurrences(of: "-", with: "")
        return "\(prefix)-\(suffix)@example.com"
    }

    private static func sqlStringLiteral(_ value: String) -> String {
        "'\(value.replacingOccurrences(of: "'", with: "''"))'"
    }
}
