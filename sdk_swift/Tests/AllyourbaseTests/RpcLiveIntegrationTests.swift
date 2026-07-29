import Foundation
import Testing
@testable import Allyourbase

@Suite("RpcLiveIntegrationTests")
struct RpcLiveIntegrationTests {
    @Test(
        .enabled(
            if: ProcessInfo.processInfo.environment["AYB_REQUIRE_RPC_LIVE"] == "true",
            "Set AYB_REQUIRE_RPC_LIVE=true to run live Swift RPC proof."
        )
    )
    func sdkContractAddReturnsCanonicalFixtureFromLiveServer() async throws {
        let client = try RecordsLiveIntegrationSupport.newClient()

        let response = try await client.rpc("sdk_contract_add", args: ContractFixtures.rpcRequest)

        let dictionary = try #require(response as? [String: Any])
        #expect(NSDictionary(dictionary: dictionary).isEqual(to: ContractFixtures.rpcResponse))
    }
}
