import Foundation
import Testing
@testable import Allyourbase

struct SearchSynonymsTests {
    @Test func searchSynonymsResponseFixtureDecodesCanonicalContract() throws {
        let response = try SearchSynonymsResponse.decode(ContractFixtures.searchSynonymsResponse)

        #expect(response.groups.count == 2)
        #expect(response.groups[0].terms == ["new york", "nyc"])
        #expect(response.groups[1].terms == ["science fiction", "scifi"])
    }

    @Test func setSynonymsPutsRequestEnvelopeWithoutRewrapping() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.searchSynonymsResponse))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)
        client.setApiKey("admin-token")
        let request = try SearchSynonymsRequest.decode(ContractFixtures.searchSynonymsRequest)

        let response = try await client.records.setSynonyms(
            "sdk_contract_synonyms_fixture",
            request: request
        )

        #expect(response.groups[0].terms == ["new york", "nyc"])
        let capturedRequest = try #require(transport.requests.last)
        #expect(capturedRequest.method.rawValue == "PUT")
        #expect(capturedRequest.url.absoluteString == "https://api.example.com/api/collections/sdk_contract_synonyms_fixture/synonyms/")
        #expect(lowercasedLookup(capturedRequest.headers, "Authorization") == "Bearer admin-token")
        #expect(lowercasedLookup(capturedRequest.headers, "Content-Type") == "application/json")

        let body = try #require(capturedRequest.body)
        let decoded = try #require(JSONSerialization.jsonObject(with: body) as? [String: Any])
        #expect(NSDictionary(dictionary: decoded).isEqual(to: ContractFixtures.searchSynonymsRequest))
    }

    @Test func getSynonymsEscapesCollectionSegmentAndDecodesResponse() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.searchSynonymsResponse))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)
        client.setApiKey("admin-token")

        let response = try await client.records.getSynonyms("posts/../../admin")

        let request = try #require(transport.requests.last)
        #expect(request.method.rawValue == "GET")
        #expect(request.url.absoluteString == "https://api.example.com/api/collections/posts%2F..%2F..%2Fadmin/synonyms/")
        #expect(lowercasedLookup(request.headers, "Authorization") == "Bearer admin-token")
        #expect(response.groups.map(\.terms) == [
            ["new york", "nyc"],
            ["science fiction", "scifi"],
        ])
    }

    @Test func getSynonymsEscapesReservedCollectionCharactersLikeEncodeURIComponent() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.searchSynonymsResponse))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        _ = try await client.records.getSynonyms("posts?draft=100%")

        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/collections/posts%3Fdraft%3D100%25/synonyms/")
    }
}
