import assert from "node:assert/strict";
import { AYBClient } from "@allyourbase/js";
import { createInstantSearchClient } from "@allyourbase/js/instantsearch";
import {
  INSTANTSEARCH_COLLECTION,
  INSTANTSEARCH_OBJECT_ID_FIELD,
  startInstantSearchRuntime,
} from "./live_runtime.mjs";

const runtime = await startInstantSearchRuntime();

try {
  const client = new AYBClient(runtime.apiURL);
  const searchClient = createInstantSearchClient({
    client,
    objectIDField: INSTANTSEARCH_OBJECT_ID_FIELD,
    highlight: true,
  });

  const defaultResponse = await searchClient.search([
    {
      indexName: INSTANTSEARCH_COLLECTION,
      params: {
        query: "",
        hitsPerPage: 6,
        facets: ["category"],
      },
    },
  ]);
  const defaultResult = singleResult(defaultResponse);

  assert.equal(defaultResult.query, "");
  assert.equal(defaultResult.page, 0);
  assert.equal(defaultResult.nbHits, 14);
  assert.equal(defaultResult.nbPages, 3);
  assert.equal(defaultResult.hitsPerPage, 6);
  assert.equal(defaultResult.hits[0].objectID, "red-notebook");
  assert.equal(defaultResult.hits[0].slug, "red-notebook");
  assert.equal(defaultResult.facets?.category?.Stationery, 3);
  assert.equal(defaultResult.facets?.category?.Lighting, 3);
  assert.equal(defaultResult.facets?.category?.Office, 3);
  assert.equal(defaultResult.facets?.category?.Kitchen, 3);
  assert.equal(defaultResult.facets?.category?.Travel, 2);

  const targetedResponse = await searchClient.search([
    {
      indexName: INSTANTSEARCH_COLLECTION,
      params: {
        query: "red",
        hitsPerPage: 6,
        facets: ["category"],
      },
    },
  ]);
  const targetedResult = singleResult(targetedResponse);
  const redNotebook = targetedResult.hits.find(
    (hit) => hit.objectID === "red-notebook",
  );

  assert.equal(targetedResult.query, "red");
  assert.equal(targetedResult.nbHits, 1);
  assert.ok(redNotebook, "expected red-notebook in targeted search results");
  assert.match(
    redNotebook._highlightResult.title.value,
    /__ais-highlight__Red__\/ais-highlight__ Notebook/,
  );
  assert.match(redNotebook._highlightResult.description.value, /crimson ledger/);
} finally {
  await runtime.stop();
}

function singleResult(response) {
  assert.equal(response.results.length, 1);
  return response.results[0];
}
