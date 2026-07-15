import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ADMIN_CAPABILITY_NAMES,
  getAdminCapabilities,
  type AdminCapabilities,
} from "../api_capabilities";

const exactCapabilities = Object.fromEntries(
  ADMIN_CAPABILITY_NAMES.map((name, index) => [name, index % 2 === 0]),
) as AdminCapabilities;

function mockResponse(status: number, body: unknown, contentType = "application/json"): Response {
  return new Response(
    typeof body === "string" ? body : JSON.stringify(body),
    {
      status,
      headers: { "Content-Type": contentType },
    },
  );
}

describe("getAdminCapabilities", () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    vi.stubGlobal("fetch", fetchMock);
    localStorage.clear();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("returns known for an exact valid Stage 2 capability payload", async () => {
    fetchMock.mockResolvedValueOnce(mockResponse(200, exactCapabilities));

    await expect(getAdminCapabilities()).resolves.toEqual({
      kind: "known",
      capabilities: exactCapabilities,
    });
  });

  it("permits extra capability keys for forward compatibility", async () => {
    fetchMock.mockResolvedValueOnce(mockResponse(200, { ...exactCapabilities, future_flag: true }));

    await expect(getAdminCapabilities()).resolves.toEqual({
      kind: "known",
      capabilities: exactCapabilities,
    });
  });

  it.each([
    ["network failure", () => fetchMock.mockRejectedValueOnce(new Error("network down"))],
    ["401", () => fetchMock.mockResolvedValueOnce(mockResponse(401, { message: "unauthorized" }))],
    ["another non-200", () => fetchMock.mockResolvedValueOnce(mockResponse(503, { message: "down" }))],
    ["non-JSON", () => fetchMock.mockResolvedValueOnce(mockResponse(200, "not json", "text/plain"))],
    ["non-object", () => fetchMock.mockResolvedValueOnce(mockResponse(200, [exactCapabilities]))],
    [
      "missing key",
      () => {
        const { auth, ...missingAuth } = exactCapabilities;
        void auth;
        fetchMock.mockResolvedValueOnce(mockResponse(200, missingAuth));
      },
    ],
    [
      "non-boolean value",
      () => fetchMock.mockResolvedValueOnce(mockResponse(200, { ...exactCapabilities, auth: "true" })),
    ],
  ])("returns unknown for %s without throwing or emitting unauthorized", async (_name, arrange) => {
    const unauthorizedListener = vi.fn();
    window.addEventListener("ayb:unauthorized", unauthorizedListener);
    arrange();

    await expect(getAdminCapabilities()).resolves.toEqual({ kind: "unknown" });

    expect(unauthorizedListener).not.toHaveBeenCalled();
    window.removeEventListener("ayb:unauthorized", unauthorizedListener);
  });
});
