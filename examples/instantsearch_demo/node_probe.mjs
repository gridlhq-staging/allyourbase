import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createServer } from "node:net";
import { AYBClient } from "@allyourbase/js";
import { createInstantSearchClient } from "@allyourbase/js/instantsearch";

const DEFAULT_HITS_PER_PAGE = 6;

await configureDefaultRuntimePorts();

const {
  INSTANTSEARCH_COLLECTION,
  INSTANTSEARCH_OBJECT_ID_FIELD,
  startInstantSearchRuntime,
} = await import("./live_runtime.mjs");

const expectedSeedTotals = await readExpectedSeedTotals();
const selectedBrands = Object.keys(expectedSeedTotals.facets.brand).slice(0, 2);
assert.equal(selectedBrands.length, 2, "expected at least two seeded brands");

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
        hitsPerPage: DEFAULT_HITS_PER_PAGE,
        facets: ["category", "brand"],
      },
    },
  ]);
  const defaultResult = singleResult(defaultResponse);

  assert.equal(defaultResult.query, "");
  assert.equal(defaultResult.page, 0);
  assert.equal(defaultResult.nbHits, expectedSeedTotals.totalHits);
  assert.equal(defaultResult.nbPages, expectedSeedTotals.defaultNbPages);
  assert.equal(defaultResult.hitsPerPage, DEFAULT_HITS_PER_PAGE);
  assert.equal(defaultResult.hits[0].objectID, "red-notebook");
  assert.equal(defaultResult.hits[0].slug, "red-notebook");
  assert.deepEqual(
    facetCounts(defaultResult, "category"),
    expectedSeedTotals.facets.category,
  );
  assert.deepEqual(facetCounts(defaultResult, "brand"), expectedSeedTotals.facets.brand);

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

  const brandFacetResponse = await searchClient.search([
    {
      indexName: INSTANTSEARCH_COLLECTION,
      params: {
        query: "",
        hitsPerPage: 10,
        facets: ["brand"],
        disjunctiveFacets: ["brand"],
        facetFilters: [selectedBrands.map((brand) => `brand:${brand}`)],
      },
    },
  ]);
  const brandFacetResult = singleResult(brandFacetResponse);
  const selectedBrandCounts = disjunctiveFacetData(brandFacetResult, "brand");
  const returnedBrands = new Set(brandFacetResult.hits.map((hit) => hit.brand));

  assert.equal(brandFacetResult.nbHits, selectedBrandHitCount(selectedBrands));
  for (const brand of selectedBrands) {
    assert.equal(selectedBrandCounts[brand], expectedSeedTotals.facets.brand[brand]);
    assert.ok(returnedBrands.has(brand), `expected ${brand} hits in brand OR result`);
  }

  const inBandResponse = await searchClient.search([
    {
      indexName: INSTANTSEARCH_COLLECTION,
      params: {
        query: "",
        hitsPerPage: 10,
        facets: ["price_cents"],
        numericFilters: [
          `price_cents>=${expectedSeedTotals.priceBand.minBound}`,
          `price_cents<=${expectedSeedTotals.priceBand.maxBound}`,
        ],
      },
    },
  ]);
  const inBandResult = singleResult(inBandResponse);

  assert.equal(inBandResult.nbHits, expectedSeedTotals.priceBand.inBand);
  assert.equal(
    inBandResult.facets_stats.price_cents.min,
    expectedSeedTotals.priceBand.min,
  );
  assert.equal(
    inBandResult.facets_stats.price_cents.max,
    expectedSeedTotals.priceBand.max,
  );

  const outOfBandResponse = await searchClient.search([
    {
      indexName: INSTANTSEARCH_COLLECTION,
      params: {
        query: "",
        hitsPerPage: 12,
        facets: ["price_cents"],
        numericFilters: [
          [
            `price_cents<${expectedSeedTotals.priceBand.minBound}`,
            `price_cents>${expectedSeedTotals.priceBand.maxBound}`,
          ],
        ],
      },
    },
  ]);
  const outOfBandResult = singleResult(outOfBandResponse);

  assert.equal(outOfBandResult.nbHits, expectedSeedTotals.priceBand.outOfBand);

  await assertRawFacetValueSearchContract(runtime.apiURL);
  await assertAdapterFacetValueSearchContract(searchClient);
} finally {
  await runtime.stop();
}

async function readExpectedSeedTotals() {
  const seedSQL = await readFile(new URL("./seed.sql", import.meta.url), "utf8");
  const totalHits = parseLabeledInteger(seedSQL, "total_hits");
  const priceBand = parsePriceBand(seedSQL, "price_cents_2000_6000");
  const seedPriceCents = seededPrices(seedSQL);
  assert.equal(
    seedPriceCents.length,
    totalHits,
    "expected total_hits to match seeded product rows",
  );
  return {
    totalHits,
    defaultNbPages: Math.ceil(totalHits / DEFAULT_HITS_PER_PAGE),
    facets: {
      category: parseFacetTotals(seedSQL, "categories"),
      brand: parseFacetTotals(seedSQL, "brands"),
    },
    priceBand: {
      ...priceBand,
      ...priceBandStats(seedPriceCents, priceBand),
    },
  };
}

function parseFacetTotals(seedSQL, label) {
  const line = commentValue(seedSQL, label);
  return Object.fromEntries(
    line.split(",").map((entry) => {
      const [name, rawCount] = entry.trim().split("=");
      assert.ok(name && rawCount, `expected ${label} entry to contain name=count`);
      return [name, parseInteger(rawCount, `${label} ${name}`)];
    }),
  );
}

function parseLabeledInteger(seedSQL, label) {
  return parseInteger(commentValue(seedSQL, label), label);
}

function parsePriceBand(seedSQL, bandLabel) {
  const line = commentValue(seedSQL, `price_range ${bandLabel}`);
  const bounds = bandLabel.match(/^price_cents_(\d+)_(\d+)$/);
  assert.ok(bounds, `expected parseable price range label: ${bandLabel}`);
  return {
    minBound: parseInteger(bounds[1], `${bandLabel} lower bound`),
    maxBound: parseInteger(bounds[2], `${bandLabel} upper bound`),
    inBand: parseKeyedCount(line, "in_band"),
    outOfBand: parseKeyedCount(line, "out_of_band"),
  };
}

function parseKeyedCount(line, key) {
  const match = line.match(new RegExp(`(?:^|, )${key}=(\\d+)(?:,|$)`));
  assert.ok(match, `expected ${key} count in price range comment`);
  return parseInteger(match[1], key);
}

function commentValue(seedSQL, label) {
  const escapedLabel = label.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const match = seedSQL.match(new RegExp(`^-- ${escapedLabel}: (.+)$`, "m"));
  assert.ok(match, `expected seed.sql comment for ${label}`);
  return match[1].trim();
}

function priceBandStats(seedPriceCents, priceBand) {
  const prices = seedPriceCents.filter(
    (price) => price >= priceBand.minBound && price <= priceBand.maxBound,
  );
  assert.equal(
    prices.length,
    priceBand.inBand,
    "expected in_band to match seeded prices in documented band",
  );
  assert.equal(
    seedPriceCents.length - prices.length,
    priceBand.outOfBand,
    "expected out_of_band to match seeded prices outside documented band",
  );
  return {
    min: Math.min(...prices),
    max: Math.max(...prices),
  };
}

function seededPrices(seedSQL) {
  return [...seedSQL.matchAll(/\((?:'[^']*',\s*){5}(\d+)\)/g)].map((match) =>
    parseInteger(match[1], "seeded price_cents"),
  );
}

function parseInteger(rawValue, label) {
  const normalizedValue = rawValue.trim();
  assert.match(normalizedValue, /^\d+$/, `expected integer for ${label}`);
  return Number.parseInt(normalizedValue, 10);
}

async function assertRawFacetValueSearchContract(apiURL) {
  const emptyResponse = await fetchFacetValueSearch(apiURL, "");
  assert.deepEqual(emptyResponse, {
    facetHits: [
      { value: "Kitchen", highlighted: "Kitchen", count: 4 },
      { value: "Lighting", highlighted: "Lighting", count: 4 },
      { value: "Office", highlighted: "Office", count: 4 },
      { value: "Stationery", highlighted: "Stationery", count: 4 },
      { value: "Travel", highlighted: "Travel", count: 4 },
    ],
    exhaustiveFacetsCount: true,
  });

  const prefixResponse = await fetchFacetValueSearch(apiURL, "st");
  assert.deepEqual(prefixResponse, {
    facetHits: [
      { value: "Stationery", highlighted: "<mark>St</mark>ationery", count: 4 },
    ],
    exhaustiveFacetsCount: true,
  });

  const singleHitResponse = await fetchFacetValueSearch(apiURL, "T");
  assert.deepEqual(singleHitResponse, {
    facetHits: [
      { value: "Travel", highlighted: "<mark>T</mark>ravel", count: 4 },
    ],
    exhaustiveFacetsCount: true,
  });
}

async function fetchFacetValueSearch(apiURL, q) {
  const url = new URL(
    `/api/collections/${INSTANTSEARCH_COLLECTION}/facets/category/search`,
    apiURL,
  );
  if (q !== "") url.searchParams.set("q", q);
  const response = await fetch(url, { signal: AbortSignal.timeout(5_000) });
  assert.equal(
    response.status,
    200,
    `facet value search returned ${response.status} for q=${JSON.stringify(q)}`,
  );
  return response.json();
}

async function assertAdapterFacetValueSearchContract(searchClient) {
  const defaultResponse = await searchClient.searchForFacetValues([
    {
      indexName: INSTANTSEARCH_COLLECTION,
      params: { facetName: "category", facetQuery: "St" },
    },
  ]);
  assert.equal(defaultResponse.length, 1);
  const [defaultResult] = defaultResponse;
  assert.equal(defaultResult.exhaustiveFacetsCount, true);
  assert.deepEqual(defaultResult.facetHits, [
    {
      value: "Stationery",
      highlighted: "__ais-highlight__St__/ais-highlight__ationery",
      count: 4,
    },
  ]);
  assert.equal(typeof defaultResult.processingTimeMS, "number");
  assert.ok(defaultResult.processingTimeMS >= 0);

  const markTagResponse = await searchClient.searchForFacetValues([
    {
      indexName: INSTANTSEARCH_COLLECTION,
      params: {
        facetName: "category",
        facetQuery: "St",
        highlightPreTag: "<mark>",
        highlightPostTag: "</mark>",
      },
    },
  ]);
  assert.deepEqual(markTagResponse[0].facetHits, [
    {
      value: "Stationery",
      highlighted: "<mark>St</mark>ationery",
      count: 4,
    },
  ]);

  const emptyQueryResponse = await searchClient.searchForFacetValues([
    {
      indexName: INSTANTSEARCH_COLLECTION,
      params: { facetName: "category", maxFacetHits: 10 },
    },
  ]);
  const [emptyResult] = emptyQueryResponse;
  assert.equal(emptyResult.exhaustiveFacetsCount, true);
  assert.deepEqual(
    emptyResult.facetHits.map((hit) => [hit.value, hit.count, hit.highlighted]),
    [
      ["Kitchen", 4, "Kitchen"],
      ["Lighting", 4, "Lighting"],
      ["Office", 4, "Office"],
      ["Stationery", 4, "Stationery"],
      ["Travel", 4, "Travel"],
    ],
  );

  const truncatedResponse = await searchClient.searchForFacetValues([
    {
      indexName: INSTANTSEARCH_COLLECTION,
      params: { facetName: "category", maxFacetHits: 2 },
    },
  ]);
  const [truncatedResult] = truncatedResponse;
  assert.equal(truncatedResult.exhaustiveFacetsCount, false);
  assert.equal(truncatedResult.facetHits.length, 2);
  assert.deepEqual(
    truncatedResult.facetHits.map((hit) => hit.value),
    ["Kitchen", "Lighting"],
  );
}

function singleResult(response) {
  assert.equal(response.results.length, 1);
  return response.results[0];
}

function facetCounts(result, facetName) {
  assert.ok(result.facets?.[facetName], `expected ${facetName} facet counts`);
  return result.facets[facetName];
}

function disjunctiveFacetData(result, facetName) {
  const facet = result.disjunctiveFacets?.find((entry) => entry.name === facetName);
  assert.ok(facet, `expected ${facetName} disjunctive facet data`);
  return facet.data;
}

function selectedBrandHitCount(brands) {
  return brands.reduce(
    (total, brand) => total + expectedSeedTotals.facets.brand[brand],
    0,
  );
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
