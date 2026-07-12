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
    func webAuthnMFABeginEndpointsReturnConcreteOptions() async throws {
        let adminClient = try RecordsLiveIntegrationSupport.newClient()
        let user = try await RecordsLiveIntegrationSupport.registerLiveUserClient(
            using: adminClient,
            emailPrefix: "swift-live-webauthn-mfa"
        )
        do {
            let enroll = try await user.client.auth.enrollWebAuthn()
            #expect(enroll.challenge.isEmpty == false)
            #expect(enroll.rp.name.isEmpty == false)
            #expect(enroll.rp.id?.isEmpty == false)
            #expect(enroll.user.id.isEmpty == false)
            #expect(enroll.user.name == user.email)
            #expect(enroll.timeout > 0)
            #expect(enroll.pubKeyCredParams.isEmpty == false)
            #expect(enroll.pubKeyCredParams.allSatisfy { $0.alg != 0 })

            try await RecordsLiveIntegrationSupport.seedWebAuthnMFAFactor(
                using: adminClient,
                email: user.email
            )
            let mfaToken = try await RecordsLiveIntegrationSupport.loginForMFAPendingToken(
                email: user.email,
                password: user.password
            )
            let challenge = try await user.client.auth.webauthnChallenge(mfaToken: mfaToken)
            #expect(challenge.challengeId.isEmpty == false)
            #expect(challenge.options.challenge.isEmpty == false)
            #expect(challenge.options.rpId.isEmpty == false)
            #expect(challenge.options.allowCredentials.isEmpty == false)
            try await RecordsLiveIntegrationSupport.deleteLiveUser(using: adminClient, email: user.email)
        } catch {
            try? await RecordsLiveIntegrationSupport.deleteLiveUser(using: adminClient, email: user.email)
            throw error
        }
    }

    @Test(
        .enabled(
            if: RecordsLiveIntegrationSupport.hasRequiredConfiguration(),
            "Set AYB_TEST_URL and AYB_TEST_ADMIN_TOKEN/AYB_ADMIN_TOKEN, or provide ~/.ayb/admin-token before running live Swift SDK tests."
        )
    )
    func beginWebAuthnLoginReturnsConcreteChallengeOptions() async throws {
        let client = try RecordsLiveIntegrationSupport.newClient()

        let response = try await client.auth.beginWebAuthnLogin(
            email: "swift-live-webauthn-decoy@example.com"
        )

        #expect(response.challengeId.isEmpty == false)
        #expect(response.options.challenge.isEmpty == false)
        #expect(response.options.rpId == "127.0.0.1")
        #expect(response.options.allowCredentials.isEmpty == false)
    }

    @Test(
        .enabled(
            if: RecordsLiveIntegrationSupport.hasRequiredConfiguration(),
            "Set AYB_TEST_URL and AYB_TEST_ADMIN_TOKEN/AYB_ADMIN_TOKEN, or provide ~/.ayb/admin-token before running live Swift SDK tests."
        )
    )
    func searchSynonymsRoundTripReturnsNormalizedGroups() async throws {
        let collection = "\(RecordsLiveIntegrationSupport.collection)_synonyms"
        let client = try RecordsLiveIntegrationSupport.newClient()
        try await RecordsLiveIntegrationSupport.prepareSearchFixtures(using: client, collection: collection)
        do {
            let expectedGroups = [
                ["ai", "artificial intelligence", "machine learning"],
                ["science fiction", "scifi"],
            ]

            let putResponse = try await client.records.setSynonyms(
                collection,
                groups: [
                    SearchSynonymGroup(terms: [" SciFi ", "Science Fiction"]),
                    SearchSynonymGroup(terms: ["AI", "Artificial Intelligence", "Machine Learning"]),
                ]
            )
            #expect(putResponse.groups.map(\.terms) == expectedGroups)

            let getResponse = try await client.records.getSynonyms(collection)
            #expect(getResponse.groups.map(\.terms) == expectedGroups)

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
