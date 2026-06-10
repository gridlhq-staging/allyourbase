import { describe, expect, it, vi } from "vitest";

const aybClientSpy = vi.fn();
const createInstantSearchClientSpy = vi.fn(() => ({
  search: vi.fn(),
  searchForFacetValues: vi.fn(),
}));

vi.mock("@allyourbase/js", () => ({
  AYBClient: aybClientSpy,
}));

vi.mock("@allyourbase/js/instantsearch", () => ({
  createInstantSearchClient: createInstantSearchClientSpy,
}));

describe("instantsearch demo AYB bootstrap", () => {
  it("constructs AYB and exports the Stage 2 adapter search client", async () => {
    const module = await import("../src/lib/ayb");

    expect(aybClientSpy).toHaveBeenCalledTimes(1);
    expect(aybClientSpy).toHaveBeenCalledWith("http://127.0.0.1:8090");
    expect(createInstantSearchClientSpy).toHaveBeenCalledTimes(1);
    expect(createInstantSearchClientSpy).toHaveBeenCalledWith({
      client: aybClientSpy.mock.instances[0],
      objectIDField: "slug",
      highlight: true,
    });
    expect(module.searchClient).toBe(createInstantSearchClientSpy.mock.results[0].value);
    expect(module.searchClient.search).toBe(createInstantSearchClientSpy.mock.results[0].value.search);
  });
});
