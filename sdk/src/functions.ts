/**
 * @module Thin method-grouping client for invoking edge functions over the shared AYBClient transport.
 */
import type { EdgeInvokeOptions, EdgeInvokeResponse } from "./types";
import { encodePathSegment } from "./helpers";

/** Runtime seam the FunctionsClient delegates to; the concrete owner is AYBClient. */
interface FunctionsClientRuntime {
  invokeEdge(path: string, options?: EdgeInvokeOptions): Promise<EdgeInvokeResponse>;
}

/**
 * Groups edge-function operations. It is a thin facade over the client's shared
 * request/auth/error seam and adds no independent transport of its own.
 */
export class FunctionsClient {
  constructor(private client: FunctionsClientRuntime) {}

  /** Invoke an edge function by name and return the raw HTTP response envelope. */
  async invoke(
    name: string,
    options?: EdgeInvokeOptions,
  ): Promise<EdgeInvokeResponse> {
    return this.client.invokeEdge(
      `/functions/v1/${encodePathSegment(name)}`,
      options,
    );
  }
}
