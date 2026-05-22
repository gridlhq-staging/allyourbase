import { AYBGraphQLError } from "./errors";
import type { GraphQLErrorItem, GraphQLResponse } from "./types";

interface GraphQLRuntime {
  request<T>(path: string, init?: RequestInit & { skipAuth?: boolean }): Promise<T>;
}

/** SDK GraphQL client built on top of AYBClient request transport. */
export class GraphQLClient {
  constructor(private readonly runtime: GraphQLRuntime) {}

  async query<TData>(
    document: string,
    variables?: Record<string, unknown>,
  ): Promise<TData> {
    const response = await this.runtime.request<GraphQLResponse<TData>>(
      "/api/graphql",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ query: document, variables }),
      },
    );

    if (response.errors && response.errors.length > 0) {
      throw new AYBGraphQLError(firstGraphQLErrorMessage(response.errors), response.errors);
    }

    return response.data as TData;
  }
}

function firstGraphQLErrorMessage(errors: GraphQLErrorItem[]): string {
  const firstMessage = errors[0]?.message;
  return firstMessage || "GraphQL request failed";
}
