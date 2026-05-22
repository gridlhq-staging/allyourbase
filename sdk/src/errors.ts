import type { GraphQLErrorItem } from "./types";

/** Error thrown when the AYB API returns a non-2xx response. */
export class AYBError extends Error {
  constructor(
    public readonly status: number,
    message: string,
    /** Machine-readable error code for programmatic handling. */
    public readonly code?: string,
    /** Field-level validation detail (e.g. constraint violations). */
    public readonly data?: Record<string, unknown>,
    /** Link to relevant documentation. */
    public readonly docUrl?: string,
  ) {
    super(message);
    this.name = "AYBError";
  }
}

/** Error thrown when a GraphQL response returns an errors array. */
export class AYBGraphQLError extends Error {
  constructor(
    message: string,
    /** GraphQL error list returned by the API response. */
    public readonly errors: GraphQLErrorItem[],
  ) {
    super(message);
    this.name = "AYBGraphQLError";
  }
}
