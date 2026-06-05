import Foundation
import Testing
@testable import Allyourbase

@Suite("RecordsLiveIntegrationTests")
struct RecordsLiveIntegrationTests {
    @Test(
        .enabled(
            if: RecordsLiveIntegrationSupport.hasRequiredConfiguration(),
            "Set AYB_TEST_URL and AYB_TEST_ADMIN_TOKEN/AYB_ADMIN_TOKEN, or provide ~/.ayb/admin-token before running live Swift SDK tests."
        )
    )
    func searchHighlightRequiresConfiguredServer() async throws {
        let collection = "\(RecordsLiveIntegrationSupport.collection)_highlight"
        let client = try RecordsLiveIntegrationSupport.newClient()
        try await RecordsLiveIntegrationSupport.prepareSearchFixtures(using: client, collection: collection)
        do {
            let response = try await client.records.list(
                collection,
                params: ListParams(search: "allyourbase", highlight: true)
            )

            let highlights = response.items.compactMap { $0["_highlight"] as? String }
            #expect(highlights.isEmpty == false)
            #expect(highlights.contains(where: { $0.contains("<b>allyourbase</b>") }))
            try await RecordsLiveIntegrationSupport.dropSearchFixtures(using: client, collection: collection)
        } catch {
            try? await RecordsLiveIntegrationSupport.dropSearchFixtures(using: client, collection: collection)
            throw error
        }
    }

    @Test(
        .enabled(
            if: RecordsLiveIntegrationSupport.hasRequiredConfiguration(),
            "Set AYB_TEST_URL and AYB_TEST_ADMIN_TOKEN/AYB_ADMIN_TOKEN, or provide ~/.ayb/admin-token before running live Swift SDK tests."
        )
    )
    func fuzzySearchMatchesTypoWhenConfiguredServerExists() async throws {
        let collection = "\(RecordsLiveIntegrationSupport.collection)_fuzzy"
        let client = try RecordsLiveIntegrationSupport.newClient()
        try await RecordsLiveIntegrationSupport.prepareSearchFixtures(using: client, collection: collection)
        do {
            let response = try await client.records.list(
                collection,
                params: ListParams(
                    search: "alyourbase",
                    fuzzy: true,
                    typoThreshold: 0.2,
                    facets: ["category"]
                )
            )

            let ids = Set(response.items.compactMap { $0["id"] as? String })
            #expect(ids.contains("one"))
            #expect(ids.contains("two"))
            let categoryFacets = response.facets?["category"] as? [[String: Any]]
            #expect(categoryFacets?.count == 1)
            #expect(categoryFacets?.first?["value"] as? String == "docs")
            #expect(categoryFacets?.first?["count"] as? Int == 2)
            try await RecordsLiveIntegrationSupport.dropSearchFixtures(using: client, collection: collection)
        } catch {
            try? await RecordsLiveIntegrationSupport.dropSearchFixtures(using: client, collection: collection)
            throw error
        }
    }
}
