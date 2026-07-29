import Foundation
import Testing
@testable import Allyourbase

struct EdgeInvokeClientTests {
    @Test func invokePostsFixtureBodyWithRequestBuilderHeaders() async throws {
        let transport = MockTransport()
        transport.enqueue(
            StubResponse(
                status: 200,
                body: ContractFixtures.edgeInvokeResponseData,
                headers: ["x-edge-run": "deterministic"]
            )
        )
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)
        client.setApiKey("edge-token")

        let response = try await client.functions.invoke(
            "sdk-contract-echo",
            options: EdgeInvokeOptions(
                headers: ["X-Trace-Id": "trace-1"],
                body: ContractFixtures.edgeInvokeRequest
            )
        )

        let request = try #require(transport.requests.last)
        #expect(request.method == .post)
        #expect(request.url.path == "/functions/v1/sdk-contract-echo")
        #expect(lowercasedLookup(request.headers, "Accept") == "application/json")
        #expect(lowercasedLookup(request.headers, "Authorization") == "Bearer edge-token")
        #expect(lowercasedLookup(request.headers, "X-Trace-Id") == "trace-1")
        #expect(lowercasedLookup(request.headers, "Content-Type") == "application/json")
        #expect(request.body == dataFromJSON(ContractFixtures.edgeInvokeRequest))
        #expect(response.status == 200)
        #expect(lowercasedLookup(response.headers, "x-edge-run") == "deterministic")
        #expect(response.rawBody == ContractFixtures.edgeInvokeResponseData)
    }

    @Test func invokeEncodesFunctionNameAsOneRFC3986PathSegment() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 204, body: Data()))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        _ = try await client.functions.invoke("schema/fn name")

        let request = try #require(transport.requests.last)
        let components = try #require(URLComponents(url: request.url, resolvingAgainstBaseURL: false))
        #expect(components.percentEncodedPath == "/functions/v1/schema%2Ffn%20name")
    }

    @Test func customTransportReceivesArbitraryCallerMethodThroughPublicRequestMethod() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 204, body: Data()))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        _ = try await client.functions.invoke(
            "sdk-contract-echo",
            options: EdgeInvokeOptions(method: "OPTIONS")
        )

        let request = try #require(transport.requests.last)
        #expect(request.method.rawValue == "OPTIONS")
    }

    @Test func skipAuthOmitsBearerAndPreservesNormalizedErrors() async throws {
        let transport = MockTransport()
        transport.enqueue(
            StubResponse(
                status: 403,
                json: [
                    "message": "edge denied",
                    "code": "edge/denied",
                    "data": ["function": "sdk-contract-echo"],
                    "doc_url": "https://allyourbase.io/docs/errors#edge-denied",
                ]
            )
        )
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)
        client.setApiKey("edge-token")

        do {
            _ = try await client.functions.invoke(
                "sdk-contract-echo",
                options: EdgeInvokeOptions(skipAuth: true)
            )
            Issue.record("expected AYBError")
        } catch let error as AYBError {
            #expect(error.status == 403)
            #expect(error.message == "edge denied")
            #expect(error.code == "edge/denied")
            #expect(error.data?["function"] as? String == "sdk-contract-echo")
            #expect(error.docUrl == "https://allyourbase.io/docs/errors#edge-denied")
        } catch {
            Issue.record("unexpected error type: \(error)")
        }
        let request = try #require(transport.requests.last)
        #expect(lowercasedLookup(request.headers, "Authorization") == nil)
    }

    @Test func invokePreserves202StatusHeadersAndRawBytes() async throws {
        let transport = MockTransport()
        transport.enqueue(
            StubResponse(
                status: 202,
                body: ContractFixtures.edgeInvokeResponseData,
                headers: ["x-edge-status": "accepted"]
            )
        )
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        let response = try await client.functions.invoke("sdk-contract-echo")

        #expect(response.status == 202)
        #expect(lowercasedLookup(response.headers, "x-edge-status") == "accepted")
        #expect(response.rawBody == ContractFixtures.edgeInvokeResponseData)
    }

    @Test func invokePreserves204HeadersWithEmptyRawBody() async throws {
        let transport = MockTransport()
        transport.enqueue(
            StubResponse(
                status: 204,
                body: Data(),
                headers: ["x-edge-empty": "true"]
            )
        )
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        let response = try await client.functions.invoke("sdk-contract-echo")

        #expect(response.status == 204)
        #expect(lowercasedLookup(response.headers, "x-edge-empty") == "true")
        #expect(response.rawBody.isEmpty)
    }
}
