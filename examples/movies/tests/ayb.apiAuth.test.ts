import { beforeEach, describe, expect, it, vi } from "vitest";
import { clearBYOKKey, embedNote, searchMovies } from "../src/lib/ayb";

describe("movies api request contract", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    sessionStorage.clear();
  });

  it("targets same-origin admin search endpoint", async () => {
    sessionStorage.setItem("ayb_token", "token-abc");
    sessionStorage.setItem("ayb_refresh_token", "refresh-abc");
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ rows: [{ slug: "inception" }] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await searchMovies("inception");

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [requestUrl, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(requestUrl).toContain("/api/admin/movies/search");
    expect(requestUrl.startsWith("http://localhost:8090")).toBeFalsy();
    expect(init.headers).toMatchObject({ "Content-Type": "application/json" });
    expect(init.headers).toMatchObject({ Authorization: "Bearer token-abc" });
  });

  it("targets same-origin admin embed endpoint", async () => {
    sessionStorage.setItem("ayb_token", "token-xyz");
    sessionStorage.setItem("ayb_refresh_token", "refresh-xyz");
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ movie_slug: "inception", embedding: [1, 2, 3] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await embedNote("hello", "inception");

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(init.headers).toMatchObject({ "Content-Type": "application/json" });
    expect(init.headers).toMatchObject({ Authorization: "Bearer token-xyz" });
  });

  it("sends auth headers when clearing a BYOK provider key", async () => {
    sessionStorage.setItem("ayb_token", "token-clear");
    sessionStorage.setItem("ayb_refresh_token", "refresh-clear");
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(null, {
        status: 204,
      }),
    );

    await clearBYOKKey("openai");

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [requestUrl, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(requestUrl).toContain("/api/admin/movies/byok/openai");
    expect(init.method).toBe("DELETE");
    expect(init.headers).toMatchObject({ "Content-Type": "application/json" });
    expect(init.headers).toMatchObject({ Authorization: "Bearer token-clear" });
  });
});
