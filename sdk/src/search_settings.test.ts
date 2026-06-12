import { describe, expect, it } from "vitest";
import {
  AYBClient,
  type SearchSettings,
  type SearchSynonymGroup,
  type SearchSynonymsRequest,
  type SearchSynonymsResponse,
} from "./index";
import { mockFetchSequence } from "./test_utils/mockFetchSequence";

describe("searchSettings", () => {
  it("getSearchSettings sends the trailing-slash collection route and returns the response unchanged", async () => {
    const fixture: SearchSettings = {
      attributes: [{ column: "title", weight: "high" }],
      customRanking: [{ column: "created_at", order: "desc" }],
    };
    const fetchFn = mockFetchSequence([{ status: 200, body: fixture }]);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });

    const result = await client.searchSettings.getSearchSettings("posts/../../admin");

    expect(result).toEqual(fixture);
    const call = fetchFn.mock.calls[0];
    expect(call[0]).toBe(
      "http://localhost:8090/api/collections/posts%2F..%2F..%2Fadmin/search-settings/",
    );
  });

  it("setSearchSettings sends auth, JSON headers, and the exact settings body", async () => {
    const settings: SearchSettings = {
      attributes: [
        { column: "title", weight: "high" },
        { column: "body", weight: "medium" },
      ],
      customRanking: [{ column: "created_at", order: "desc" }],
    };
    const fetchFn = mockFetchSequence([{ status: 200, body: settings }]);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setApiKey("secret-token");

    const result = await client.searchSettings.setSearchSettings("posts", settings);

    expect(result).toEqual(settings);
    const call = fetchFn.mock.calls[0];
    expect(call[0]).toBe("http://localhost:8090/api/collections/posts/search-settings/");
    expect(call[1]?.method).toBe("PUT");
    expect(call[1]?.headers).toMatchObject({
      Authorization: "Bearer secret-token",
      "Content-Type": "application/json",
    });
    expect(call[1]?.body).toBe(JSON.stringify(settings));
  });

  it("getSynonyms sends the trailing-slash synonyms route with bearer auth", async () => {
    const fixture: SearchSynonymsResponse = {
      groups: [{ terms: ["science fiction", "scifi"] }],
    };
    const fetchFn = mockFetchSequence([{ status: 200, body: fixture }]);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setApiKey("secret-token");

    const result = await client.searchSettings.getSynonyms("posts/../../admin");

    expect(result).toEqual(fixture);
    const call = fetchFn.mock.calls[0];
    expect(call[0]).toBe(
      "http://localhost:8090/api/collections/posts%2F..%2F..%2Fadmin/synonyms/",
    );
    expect(call[1]?.headers).toMatchObject({
      Authorization: "Bearer secret-token",
    });
  });

  it("setSynonyms sends auth, JSON headers, and the exact groups envelope body", async () => {
    const groups: SearchSynonymGroup[] = [
      { terms: ["scifi", "science fiction"] },
      { terms: ["nyc", "new york"] },
    ];
    const fixture: SearchSynonymsResponse = {
      groups: [
        { terms: ["science fiction", "scifi"] },
        { terms: ["new york", "nyc"] },
      ],
    };
    const fetchFn = mockFetchSequence([{ status: 200, body: fixture }]);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setApiKey("secret-token");

    const result = await client.searchSettings.setSynonyms("posts", groups);

    expect(result).toEqual(fixture);
    const call = fetchFn.mock.calls[0];
    expect(call[0]).toBe("http://localhost:8090/api/collections/posts/synonyms/");
    expect(call[1]?.method).toBe("PUT");
    expect(call[1]?.headers).toMatchObject({
      Authorization: "Bearer secret-token",
      "Content-Type": "application/json",
    });
    expect(call[1]?.body).toBe(JSON.stringify({ groups }));
  });

  it("setSynonyms accepts the exported request envelope without re-wrapping it", async () => {
    const request: SearchSynonymsRequest = {
      groups: [{ terms: ["scifi", "science fiction"] }],
    };
    const fetchFn = mockFetchSequence([{ status: 200, body: request }]);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setApiKey("secret-token");

    const result = await client.searchSettings.setSynonyms("posts", request);

    expect(result).toEqual(request);
    const call = fetchFn.mock.calls[0];
    expect(call[1]?.body).toBe(JSON.stringify(request));
  });
});
