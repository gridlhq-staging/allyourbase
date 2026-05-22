import { describe, expect, it, vi } from "vitest";
import { AYBClient } from "./client";
import { AYBError, AYBGraphQLError } from "./errors";
import { mockFetchSequence } from "./test_utils/mockFetchSequence";

describe("GraphQLClient", () => {
  it("query unwraps data on successful GraphQL response", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: { data: { polls: [{ id: "poll_1", question: "Q?" }] } },
      },
    ]);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });

    const result = await client.graphql.query<{ polls: Array<{ id: string; question: string }> }>(
      "query Polls { polls { id question } }",
    );

    expect(result).toEqual({ polls: [{ id: "poll_1", question: "Q?" }] });
  });

  it("query throws AYBGraphQLError for HTTP 200 responses that include GraphQL errors", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: {
          errors: [{ message: "Cannot query field \"unknown\" on type \"Query\"." }],
        },
      },
    ]);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });

    await expect(client.graphql.query("query Bad { unknown }")).rejects.toBeInstanceOf(AYBGraphQLError);
  });

  it("query preserves AYBError for non-2xx /api/graphql responses", async () => {
    const fetchFn = mockFetchSequence([
      { status: 500, body: { message: "graphql endpoint failed" } },
    ]);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });

    await expect(client.graphql.query("query Polls { polls { id } }")).rejects.toBeInstanceOf(AYBError);
  });

  it("query forwards variables and auth via AYBClient.request path", async () => {
    const fetchFn = mockFetchSequence([
      { status: 200, body: { data: { poll: { id: "poll_1" } } } },
    ]);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("token_123", "refresh_123");

    await client.graphql.query<{ poll: { id: string } }>(
      "query Poll($id: ID!) { poll(id: $id) { id } }",
      { id: "poll_1" },
    );

    const [url, requestInit] = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toBe("http://localhost:8090/api/graphql");
    expect(requestInit.method).toBe("POST");
    expect(requestInit.headers["Content-Type"]).toBe("application/json");
    expect(requestInit.headers.Authorization).toBe("Bearer token_123");
    expect(JSON.parse(requestInit.body as string)).toEqual({
      query: "query Poll($id: ID!) { poll(id: $id) { id } }",
      variables: { id: "poll_1" },
    });
  });
});
