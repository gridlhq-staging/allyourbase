import Foundation
import Testing

struct LiveIntegrationSupportTests {
    @Test
    func resolvesBaseURLByTrimmingWhitespaceAndTrailingSlash() {
        let baseURL = RecordsLiveIntegrationSupport.resolvedBaseURL(
            environment: ["AYB_TEST_URL": " https://api.example.com/ "]
        )

        #expect(baseURL == "https://api.example.com")
    }

    @Test
    func prefersExplicitAdminTokenOverTokenFile() async throws {
        let tempDir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: tempDir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: tempDir) }

        let tokenDir = tempDir.appendingPathComponent(".ayb", isDirectory: true)
        try FileManager.default.createDirectory(at: tokenDir, withIntermediateDirectories: true)
        let tokenPath = tokenDir.appendingPathComponent("admin-token")
        try "file-token".write(to: tokenPath, atomically: true, encoding: .utf8)

        let token = RecordsLiveIntegrationSupport.resolvedAdminToken(
            environment: [
                "AYB_TEST_ADMIN_TOKEN": "env-token",
                "HOME": tempDir.path,
            ]
        )

        #expect(token == "env-token")
    }

    @Test
    func fallsBackToAdminTokenFile() async throws {
        let tempDir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: tempDir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: tempDir) }

        let tokenDir = tempDir.appendingPathComponent(".ayb", isDirectory: true)
        try FileManager.default.createDirectory(at: tokenDir, withIntermediateDirectories: true)
        let tokenPath = tokenDir.appendingPathComponent("admin-token")
        try " file-token \n".write(to: tokenPath, atomically: true, encoding: .utf8)

        let token = RecordsLiveIntegrationSupport.resolvedAdminToken(
            environment: ["HOME": tempDir.path]
        )

        #expect(token == "file-token")
    }

    @Test
    func hasRequiredConfigurationIsFalseWhenBaseURLIsMissing() {
        #expect(RecordsLiveIntegrationSupport.hasRequiredConfiguration(environment: [:]) == false)
    }

    @Test
    func hasRequiredConfigurationIsFalseWhenAdminTokenIsMissing() {
        #expect(
            RecordsLiveIntegrationSupport.hasRequiredConfiguration(
                environment: ["AYB_TEST_URL": "http://127.0.0.1:8096"]
            ) == false
        )
    }

    @Test
    func rejectsUnsafeCollectionIdentifiers() throws {
        #expect(throws: LiveIntegrationSupportError.self) {
            _ = try RecordsLiveIntegrationSupport.validatedCollectionIdentifier(
                "sdk_swift_search_posts;DROP"
            )
        }
        #expect(throws: LiveIntegrationSupportError.self) {
            _ = try RecordsLiveIntegrationSupport.validatedCollectionIdentifier(
                "sdk-swift-search-posts"
            )
        }
    }
}
