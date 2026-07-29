import Foundation
import Testing
@testable import Allyourbase

@Suite("EdgeInvokeLiveIntegrationTests")
struct EdgeInvokeLiveIntegrationTests {
    @Test(
        .enabled(
            if: ProcessInfo.processInfo.environment["AYB_REQUIRE_EDGE_LIVE"] == "true",
            "Set AYB_REQUIRE_EDGE_LIVE=true to run live Swift edge proof."
        )
    )
    func sdkContractEchoReturnsCanonicalRawFixtureFromLiveServer() async throws {
        let client = try RecordsLiveIntegrationSupport.newClient()

        let response = try await client.functions.invoke(
            "sdk-contract-echo",
            options: EdgeInvokeOptions(body: ContractFixtures.edgeInvokeRequest)
        )

        #expect(response.status == 200)
        #expect(response.rawBody == ContractFixtures.edgeInvokeResponseData)
    }
}
