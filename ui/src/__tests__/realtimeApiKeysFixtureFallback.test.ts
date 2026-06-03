import { describe, expect, it, vi } from "vitest";
import { createApiKeyForUser } from "../../browser-tests-unmocked/fixtures/realtime";

function makeResponse(ok: boolean, status: number, statusText: string, body: unknown) {
  return {
    ok: () => ok,
    status: () => status,
    statusText: () => statusText,
    json: async () => body,
  };
}

describe("createApiKeyForUser", () => {
  it("retries with trailing slash when first admin api-keys path returns 404", async () => {
    const post = vi
      .fn()
      .mockResolvedValueOnce(makeResponse(false, 404, "Not Found", {}))
      .mockResolvedValueOnce(makeResponse(true, 201, "Created", { key: "ayb_test_key" }));
    const request = { post } as unknown as import("@playwright/test").APIRequestContext;

    const result = await createApiKeyForUser(
      request,
      "admin-token",
      "00000000-0000-0000-0000-000000000001",
      "realtime-smoke-key",
    );

    expect(result).toEqual({ key: "ayb_test_key" });
    expect(post).toHaveBeenCalledTimes(2);
    expect(post).toHaveBeenNthCalledWith(
      1,
      "/api/admin/api-keys",
      expect.objectContaining({
        data: expect.objectContaining({
          name: "realtime-smoke-key",
        }),
      }),
    );
    expect(post).toHaveBeenNthCalledWith(
      2,
      "/api/admin/api-keys/",
      expect.objectContaining({
        data: expect.objectContaining({
          name: "realtime-smoke-key",
        }),
      }),
    );
  });
});
