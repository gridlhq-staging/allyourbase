import assert from "node:assert/strict";
import { createServer } from "node:net";
import { AYBClient } from "@allyourbase/js";
import { createInstantSearchClient } from "@allyourbase/js/instantsearch";

await configureDefaultRuntimePorts();

const {
  INSTANTSEARCH_COLLECTION,
  INSTANTSEARCH_OBJECT_ID_FIELD,
  startInstantSearchRuntime,
} = await import("./live_runtime.mjs");
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

  const rangedResponse = await searchClient.search([
    {
      indexName: INSTANTSEARCH_COLLECTION,
      params: {
        query: "",
        hitsPerPage: 6,
        facets: ["category", "price_cents"],
        numericFilters: ["price_cents>=4000", "price_cents<=5000"],
      },
    },
  ]);
  const rangedResult = singleResult(rangedResponse);

  assert.equal(rangedResult.nbHits, 1);
  assert.equal(rangedResult.hits[0].objectID, "brass-desk-lamp");
  assert.equal(rangedResult.facets_stats.price_cents.min, 4599);
  assert.equal(rangedResult.facets_stats.price_cents.max, 4599);
} finally {
  await runtime.stop();
}

function singleResult(response) {
  assert.equal(response.results.length, 1);
  return response.results[0];
}

async function configureDefaultRuntimePorts() {
  if (process.env.AYB_API_PORT?.trim()) return;
  process.env.AYB_API_PORT = String(await findAvailableAPIPort());
}

async function findAvailableAPIPort() {
  const firstCandidate = 8090;
  if (await runtimePortsAvailable(firstCandidate)) return firstCandidate;

  for (let port = 18_090; port < 19_090; port += 1) {
    if (await runtimePortsAvailable(port)) return port;
  }
  throw new Error("No available API/managed Postgres port pair for node probe");
}

async function runtimePortsAvailable(apiPort) {
  const ports = process.env.AYB_DATABASE_EMBEDDED_PORT?.trim()
    ? [apiPort]
    : [apiPort, apiPort + 2];
  const availability = await Promise.all(ports.map(portAvailable));
  return availability.every(Boolean);
}

function portAvailable(port) {
  return new Promise((resolve) => {
    const server = createServer();
    server.once("error", () => resolve(false));
    server.listen(port, "127.0.0.1", () => {
      server.close(() => resolve(true));
    });
  });
}
