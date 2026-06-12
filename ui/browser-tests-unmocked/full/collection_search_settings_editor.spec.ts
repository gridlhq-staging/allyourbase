import {
  test,
  expect,
  assertSafeSQLIdentifier,
  buildParallelSafeRunID,
  dropTableIfExists,
  execSQL,
  replaceCollectionSearchSettings,
  seedRecord,
  waitForDashboard,
} from "../fixtures";

type DashboardPage = Parameters<typeof waitForDashboard>[0];
type APIRequest = Parameters<typeof dropTableIfExists>[0];

interface CleanupTarget {
  tableName: string;
  schemaName?: string;
}

async function deleteSearchSettings(
  request: APIRequest,
  adminToken: string,
  tableName: string,
  schemaName = "public",
): Promise<void> {
  await execSQL(
    request,
    adminToken,
    `
      DELETE FROM _ayb_search_settings
      WHERE schema_name = '${schemaName}' AND table_name = '${tableName}';
    `,
  );
}

async function recreateSearchableCollection(
  request: APIRequest,
  adminToken: string,
  tableName: string,
): Promise<void> {
  const safeTableName = assertSafeSQLIdentifier(tableName, "collection search settings test table");
  await dropTableIfExists(request, adminToken, safeTableName, "collection search settings test table");
  await deleteSearchSettings(request, adminToken, safeTableName);
  await execSQL(
    request,
    adminToken,
    `
      CREATE TABLE ${safeTableName} (
        id BIGSERIAL PRIMARY KEY,
        title TEXT NOT NULL,
        description TEXT NOT NULL,
        published_at TIMESTAMPTZ NOT NULL,
        page_count INTEGER NOT NULL
      );
    `,
  );
}

async function recreateDuplicateCollectionPair(
  request: APIRequest,
  adminToken: string,
  schemaName: string,
  tableName: string,
): Promise<void> {
  const safeSchemaName = assertSafeSQLIdentifier(schemaName, "collection search settings schema");
  const safeTableName = assertSafeSQLIdentifier(tableName, "collection search settings duplicate table");
  await execSQL(request, adminToken, `DROP SCHEMA IF EXISTS ${safeSchemaName} CASCADE`);
  await dropTableIfExists(request, adminToken, safeTableName, "collection search settings duplicate table");
  await deleteSearchSettings(request, adminToken, safeTableName);
  await deleteSearchSettings(request, adminToken, safeTableName, safeSchemaName);
  await execSQL(
    request,
    adminToken,
    `
      CREATE TABLE ${safeTableName} (
        id BIGSERIAL PRIMARY KEY,
        title TEXT NOT NULL
      );
      CREATE SCHEMA ${safeSchemaName};
      CREATE TABLE ${safeSchemaName}.${safeTableName} (
        id BIGSERIAL PRIMARY KEY,
        title TEXT NOT NULL
      );
    `,
  );
}

async function openCollectionSearchSettings(
  page: DashboardPage,
  tableLabel: string,
  headingLabel = tableLabel,
): Promise<void> {
  await page.goto("/admin/");
  await waitForDashboard(page);

  const sidebar = page.getByRole("complementary");
  await expect(sidebar.getByRole("button", { name: tableLabel, exact: true })).toBeVisible({
    timeout: 10000,
  });
  await sidebar.getByRole("button", { name: tableLabel, exact: true }).click();

  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: headingLabel, exact: true })).toBeVisible({
    timeout: 10000,
  });

  const searchSettingsTab = main.getByRole("button", { name: /^Search Settings$/ });
  await expect(searchSettingsTab).toBeVisible({ timeout: 10000 });
  await searchSettingsTab.click();

  await expect(
    main.getByRole("heading", { name: `Search settings for ${tableLabel}` }),
  ).toBeVisible({ timeout: 10000 });
}

function attributeRow(page: DashboardPage, rowNumber: number) {
  return page.getByRole("group", { name: `Searchable attribute ${rowNumber}` });
}

function rankingRow(page: DashboardPage, rowNumber: number) {
  return page.getByRole("group", { name: `Custom ranking row ${rowNumber}` });
}

async function expectSearchSettingsValues(
  page: DashboardPage,
  expected: {
    attributes: { column: string; weight: string }[];
    customRanking: { column: string; order: string }[];
  },
): Promise<void> {
  for (const [index, attribute] of expected.attributes.entries()) {
    const row = attributeRow(page, index + 1);
    await expect(row.getByRole("combobox", { name: `Column for attribute ${index + 1}` })).toHaveValue(
      attribute.column,
    );
    await expect(row.getByRole("combobox", { name: `Weight for ${attribute.column}` })).toHaveValue(
      attribute.weight,
    );
  }
  await expect(page.getByRole("group", { name: /^Searchable attribute \d+$/ })).toHaveCount(
    expected.attributes.length,
  );

  for (const [index, ranking] of expected.customRanking.entries()) {
    const row = rankingRow(page, index + 1);
    await expect(row.getByRole("combobox", { name: `Ranking column for row ${index + 1}` })).toHaveValue(
      ranking.column,
    );
    await expect(row.getByRole("combobox", { name: `Ranking order for row ${index + 1}` })).toHaveValue(
      ranking.order,
    );
  }
  await expect(page.getByRole("group", { name: /^Custom ranking row \d+$/ })).toHaveCount(
    expected.customRanking.length,
  );
}

test.describe("Collection Search Settings Editor (Full E2E)", () => {
  const cleanupTargetsByTestID = new Map<string, CleanupTarget[]>();

  function mainRegion(page: DashboardPage) {
    return page.getByRole("main");
  }

  function registerCleanupTarget(testID: string, target: CleanupTarget) {
    const targets = cleanupTargetsByTestID.get(testID) ?? [];
    targets.push(target);
    cleanupTargetsByTestID.set(testID, targets);
  }

  test.afterEach(async ({ request, adminToken }, testInfo) => {
    const cleanupTargets = cleanupTargetsByTestID.get(testInfo.testId) ?? [];
    cleanupTargetsByTestID.delete(testInfo.testId);

    for (const target of cleanupTargets) {
      if (target.schemaName) {
        const safeSchemaName = assertSafeSQLIdentifier(
          target.schemaName,
          "collection search settings cleanup schema",
        );
        await execSQL(request, adminToken, `DROP SCHEMA IF EXISTS ${safeSchemaName} CASCADE`).catch(
          () => {},
        );
      }
      await dropTableIfExists(
        request,
        adminToken,
        target.tableName,
        "collection search settings cleanup table",
      ).catch(() => {});
    }
  });

  test("renders seeded weighted attributes and custom ranking from the admin contract", async ({
    page,
    request,
    adminToken,
  }, testInfo) => {
    const runID = buildParallelSafeRunID(testInfo);
    const tableName = `collection_search_settings_seed_${runID}`;
    registerCleanupTarget(testInfo.testId, { tableName });

    await recreateSearchableCollection(request, adminToken, tableName);
    await seedRecord(request, adminToken, tableName, {
      title: "Seeded search settings",
      description: "Seeded relevance text",
      published_at: "2026-06-11T00:00:00Z",
      page_count: 10,
    });
    await replaceCollectionSearchSettings(request, adminToken, tableName, {
      attributes: [
        { column: "title", weight: "high" },
        { column: "description", weight: "low" },
      ],
      customRanking: [
        { column: "published_at", order: "desc" },
        { column: "page_count", order: "asc" },
      ],
    });

    await openCollectionSearchSettings(page, tableName);

    await expect(mainRegion(page).getByRole("button", { name: "Save", exact: true })).toBeDisabled();
    await expectSearchSettingsValues(page, {
      attributes: [
        { column: "title", weight: "high" },
        { column: "description", weight: "low" },
      ],
      customRanking: [
        { column: "published_at", order: "desc" },
        { column: "page_count", order: "asc" },
      ],
    });
  });

  test("saves full replacement settings and persists them after reopening", async ({
    page,
    request,
    adminToken,
  }, testInfo) => {
    const runID = buildParallelSafeRunID(testInfo);
    const tableName = `collection_search_settings_save_${runID}`;
    registerCleanupTarget(testInfo.testId, { tableName });

    await recreateSearchableCollection(request, adminToken, tableName);
    await seedRecord(request, adminToken, tableName, {
      title: "Initial search settings",
      description: "Initial relevance text",
      published_at: "2026-06-10T00:00:00Z",
      page_count: 20,
    });
    await replaceCollectionSearchSettings(request, adminToken, tableName, {
      attributes: [{ column: "title", weight: "high" }],
      customRanking: [{ column: "published_at", order: "desc" }],
    });

    await openCollectionSearchSettings(page, tableName);

    const main = mainRegion(page);
    await main.getByRole("button", { name: "Add attribute" }).click();
    const secondAttribute = attributeRow(page, 2);
    await secondAttribute.getByRole("combobox", { name: "Column for attribute 2" }).selectOption("description");
    await secondAttribute.getByRole("combobox", { name: "Weight for description" }).selectOption("medium");

    await main.getByRole("button", { name: "Add ranking" }).click();
    const secondRanking = rankingRow(page, 2);
    await secondRanking.getByRole("combobox", { name: "Ranking column for row 2" }).selectOption("page_count");
    await secondRanking.getByRole("combobox", { name: "Ranking order for row 2" }).selectOption("asc");

    await main.getByRole("button", { name: "Save", exact: true }).click();
    await expect(main.getByRole("status")).toHaveText("Saved search settings.");

    await page.reload();
    await openCollectionSearchSettings(page, tableName);
    await expectSearchSettingsValues(page, {
      attributes: [
        { column: "title", weight: "high" },
        { column: "description", weight: "medium" },
      ],
      customRanking: [
        { column: "published_at", order: "desc" },
        { column: "page_count", order: "asc" },
      ],
    });
  });

  test("renders a non-public duplicate-name collection as read-only", async ({
    page,
    request,
    adminToken,
  }, testInfo) => {
    const runID = buildParallelSafeRunID(testInfo);
    const schemaName = `search_settings_private_${runID}`;
    const tableName = `search_settings_duplicate_${runID}`;
    const nonPublicLabel = `${schemaName}.${tableName}`;
    registerCleanupTarget(testInfo.testId, { tableName, schemaName });

    await recreateDuplicateCollectionPair(request, adminToken, schemaName, tableName);

    await openCollectionSearchSettings(page, nonPublicLabel);

    await expect(
      page.getByText(`The unqualified admin route cannot safely target ${nonPublicLabel}`),
    ).toBeVisible();
    const main = mainRegion(page);
    await expect(main.getByRole("button", { name: "Save", exact: true })).toBeHidden();
    await expect(main.getByRole("button", { name: "Add attribute" })).toBeHidden();
    await expect(main.getByRole("button", { name: "Add ranking" })).toBeHidden();
  });
});
