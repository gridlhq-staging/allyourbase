import Foundation
import Testing
@testable import Allyourbase

struct RpcClientTests {
    @Test func rpcPostsFixtureBodyThroughRequestSeamAndReturnsParsedFixture() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.rpcResponse))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)
        client.setApiKey("rpc-token")

        let response = try await client.rpc("sdk_contract_add", args: ContractFixtures.rpcRequest)

        let dictionary = try #require(response as? [String: Any])
        #expect(NSDictionary(dictionary: dictionary).isEqual(to: ContractFixtures.rpcResponse))
        let request = try #require(transport.requests.last)
        #expect(request.method.rawValue == "POST")
        #expect(request.url.path == "/api/rpc/sdk_contract_add")
        #expect(lowercasedLookup(request.headers, "Authorization") == "Bearer rpc-token")
        #expect(lowercasedLookup(request.headers, "Accept") == "application/json")
        #expect(lowercasedLookup(request.headers, "Content-Type") == "application/json")

        let body = try #require(request.body)
        let decoded = try #require(JSONSerialization.jsonObject(with: body) as? [String: Any])
        #expect(NSDictionary(dictionary: decoded).isEqual(to: ContractFixtures.rpcRequest))
    }

    @Test func rpcEncodesFunctionNameAsOneRFC3986PathSegment() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 204, body: Data()))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        _ = try await client.rpc("schema/fn name")

        let request = try #require(transport.requests.last)
        let components = try #require(URLComponents(url: request.url, resolvingAgainstBaseURL: false))
        #expect(components.percentEncodedPath == "/api/rpc/schema%2Ffn%20name")
    }

    @Test func rpcReturnsNilForEmptySuccessResponses() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 204, body: Data()))
        transport.enqueue(StubResponse(status: 200, body: Data()))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        let noContent = try await client.rpc("empty_204")
        let emptyOK = try await client.rpc("empty_200")

        #expect(noContent == nil)
        #expect(emptyOK == nil)
    }

    @Test func rpcNon2xxPropagatesNormalizedAYBError() async throws {
        let transport = MockTransport()
        transport.enqueue(
            StubResponse(
                status: 422,
                json: [
                    "message": "invalid rpc arguments",
                    "code": "rpc/invalid-arguments",
                ]
            )
        )
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        do {
            _ = try await client.rpc("sdk_contract_add", args: ContractFixtures.rpcRequest)
            Issue.record("expected AYBError")
        } catch let error as AYBError {
            #expect(error.status == 422)
            #expect(error.message == "invalid rpc arguments")
            #expect(error.code == "rpc/invalid-arguments")
        } catch {
            Issue.record("unexpected error type: \(error)")
        }
    }
}
