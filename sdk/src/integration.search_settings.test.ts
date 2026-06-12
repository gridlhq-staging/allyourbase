import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { AYBClient } from "./client";
import type {
  SearchSettings,
  SearchSynonymGroup,
  SearchSynonymsResponse,
} from "./types";
import {
  INTEGRATION_RUN_ID,
  adminSql,
  createTestClient,
  dropTableAndAssertRemoved,
  getAdminToken,
  primeIntegrationSuite,
  sqlStringLiteral,
  waitForCollectionSchemaCache,
} from "./integration-helpers";

describe("SDK integration: search settings + synonyms round trip", () => {
  let client: AYBClient;
  const tableName = `sdk_search_settings_${INTEGRATION_RUN_ID}`;

  beforeAll(async () => {
    await primeIntegrationSuite();
    client = createTestClient();
    client.setApiKey(await getAdminToken());

    await adminSql(
      `CREATE TABLE ${tableName} (
        id serial PRIMARY KEY,
        title text NOT NULL,
        body text NOT NULL,
        priority int DEFAULT 0,
        created_at timestamptz DEFAULT now()
      )`,
    );
    await adminSql(`ALTER TABLE ${tableName} ENABLE ROW LEVEL SECURITY`);
    await adminSql(
      `CREATE POLICY sdk_test_all ON ${tableName} FOR ALL USING (true) WITH CHECK (true)`,
    );
    await waitForCollectionSchemaCache(client, tableName, "search settings");
  }, 60_000);

  afterAll(async () => {
    if (!client) return;
    await adminSql(
      `DELETE FROM _ayb_search_settings WHERE schema_name = 'public' AND table_name = ${sqlStringLiteral(tableName)}`,
    );
    await adminSql(
      `DELETE FROM _ayb_search_synonyms WHERE schema_name = 'public' AND table_name = ${sqlStringLiteral(tableName)}`,
    );
    await dropTableAndAssertRemoved(tableName);
  }, 35_000);

  it("setSearchSettings persists attributes + customRanking, getSearchSettings returns the same payload", async () => {
    const settings: SearchSettings = {
      attributes: [
        { column: "title", weight: "high" },
        { column: "body", weight: "medium" },
      ],
      customRanking: [
        { column: "priority", order: "desc" },
        { column: "created_at", order: "asc" },
      ],
    };

    const written = await client.searchSettings.setSearchSettings(
      tableName,
      settings,
    );
    expect(written).toEqual(settings);

    const read = await client.searchSettings.getSearchSettings(tableName);
    expect(read).toEqual(settings);
    expect(read.attributes).toEqual([
      { column: "title", weight: "high" },
      { column: "body", weight: "medium" },
    ]);
    expect(read.customRanking).toEqual([
      { column: "priority", order: "desc" },
      { column: "created_at", order: "asc" },
    ]);
  });

  it("setSynonyms normalizes terms (trim + lowercase + sort) and sorts groups; getSynonyms returns the same envelope", async () => {
    const input: SearchSynonymGroup[] = [
      { terms: ["  SciFi", "Science Fiction"] },
      { terms: ["NYC", " new york "] },
    ];
    const normalized: SearchSynonymsResponse = {
      groups: [
        { terms: ["new york", "nyc"] },
        { terms: ["science fiction", "scifi"] },
      ],
    };

    const written = await client.searchSettings.setSynonyms(tableName, input);
    expect(written).toEqual(normalized);
    expect(written.groups).toEqual(normalized.groups);
    expect(written.groups.map((group) => group.terms)).toEqual([
      ["new york", "nyc"],
      ["science fiction", "scifi"],
    ]);

    const read = await client.searchSettings.getSynonyms(tableName);
    expect(read).toEqual(normalized);
    expect(read.groups.map((group) => group.terms)).toEqual([
      ["new york", "nyc"],
      ["science fiction", "scifi"],
    ]);
  });
});
