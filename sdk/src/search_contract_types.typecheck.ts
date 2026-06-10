import type { SearchHit } from "./index";
import {
  createInstantSearchClient,
  type InstantSearchClient,
  type InstantSearchResponse,
  type InstantSearchSearchRequest,
} from "./instantsearch";

const highlightedHit: SearchHit<{ id: string; title: string }> = {
  id: "rec_1",
  title: "Postgres guide",
  _highlight: "<b>Postgres</b> guide",
  _highlightResult: {
    title: {
      value: "<b>Postgres</b> guide",
      matchLevel: "full",
    },
  },
};

const titleHighlight: { value: string; matchLevel: string } | undefined = highlightedHit._highlightResult?.title;

void titleHighlight;

const instantSearchRequest: InstantSearchSearchRequest = {
  indexName: "posts",
  params: {
    query: "postgres",
    page: 0,
    hitsPerPage: 10,
    facets: ["status"],
  },
};

const instantSearchClient: InstantSearchClient = createInstantSearchClient({
  client: {
    records: {
      list: async () => ({
        items: [],
        page: 1,
        perPage: 10,
        totalItems: 0,
        totalPages: 0,
      }),
    },
  },
  objectIDField: "id",
  defaultIndexName: "posts",
});

const instantSearchResponse: Promise<InstantSearchResponse> = instantSearchClient.search([
  instantSearchRequest,
]);

void instantSearchResponse;
