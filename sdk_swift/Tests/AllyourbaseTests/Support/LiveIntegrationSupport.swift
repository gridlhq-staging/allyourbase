import Foundation
@testable import Allyourbase

enum LiveIntegrationSupportError: Error, CustomStringConvertible {
    case missingConfiguration(String)
    case timeout(String)
    case invalidCollectionIdentifier(String)

    var description: String {
        switch self {
        case let .missingConfiguration(message):
            return message
        case let .timeout(message):
            return message
        case let .invalidCollectionIdentifier(message):
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

    private static func adminSQL(_ client: AYBClient, _ query: String) async throws {
        _ = try await client.request(
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
}
