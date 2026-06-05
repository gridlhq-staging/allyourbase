import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ayb, clearBYOKKey, embedNote, searchMovies } from "../src/lib/ayb";

describe("movies api request contract", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    sessionStorage.clear();
    ayb.clearTokens();
  });

  afterEach(() => {
    ayb.clearTokens();
  });

  it("targets canonical SDK collection list endpoint with search params", async () => {
    ayb.setTokens("token-abc", "refresh-abc");
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({
          items: [
            {
              slug: "inception",
              title: "Inception",
              overview: "",
              release_year: 2010,
              primary_genre: "Sci-Fi",
            },
          ],
          page: 1,
          perPage: 10,
          totalItems: 1,
          totalPages: 1,
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    await searchMovies({ search: "inception" });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [requestUrl, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(requestUrl).toContain("/api/collections/movies");
    expect(requestUrl).toContain("search=inception");
    expect(requestUrl).toContain("highlight=true");
    expect(requestUrl).toContain("fuzzy=true");
    expect(requestUrl).toContain("facets=primary_genre");
    expect(requestUrl.startsWith("http://localhost:8090")).toBeFalsy();
    expect(init.headers).toMatchObject({ Authorization: "Bearer token-abc" });
  });

  it("forwards filter and perPage params to the SDK list call", async () => {
    ayb.setTokens("token-abc", "refresh-abc");
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(
        JSON.stringify({ items: [], page: 1, perPage: 25, totalItems: 0, totalPages: 0 }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    await searchMovies({
      search: "",
      filter: "primary_genre='Drama' AND release_year>=2010 AND release_year<2020",
      perPage: 25,
    });

    const [requestUrl] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(requestUrl).toContain("perPage=25");
    expect(requestUrl).not.toContain("fuzzy=true");
    expect(requestUrl).not.toContain("highlight=true");
    // URLSearchParams encodes spaces as '+'; decode the entire query for an
    // unambiguous comparison.
    const query = requestUrl.split("?")[1] ?? "";
    const params = new URLSearchParams(query);
    expect(params.get("filter")).toBe(
      "primary_genre='Drama' AND release_year>=2010 AND release_year<2020",
    );
  });

  it("targets same-origin admin embed endpoint", async () => {
    sessionStorage.setItem("ayb_token", "token-xyz");
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
