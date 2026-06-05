import {
  test,
  expect,
  assertSafeSQLIdentifier,
  buildParallelSafeRunID,
  dropTableIfExists,
  execSQL,
  replaceCollectionSearchSynonyms,
  seedRecord,
  waitForDashboard,
} from "../fixtures";

type DashboardPage = Parameters<typeof waitForDashboard>[0];
type APIRequest = Parameters<typeof dropTableIfExists>[0];

const CLOSEOUT_COLLECTION = "collection_synonyms_closeout";

async function recreateSearchableCollection(
  request: APIRequest,
  adminToken: string,
  tableName: string,
): Promise<void> {
  const safeTableName = assertSafeSQLIdentifier(tableName, "collection synonyms test table");
  await dropTableIfExists(request, adminToken, safeTableName, "collection synonyms test table");
  await execSQL(
    request,
    adminToken,
    `
      DELETE FROM _ayb_search_synonyms
      WHERE schema_name = 'public' AND table_name = '${safeTableName}';
    `,
  );
  await execSQL(
    request,
    adminToken,
    `
      CREATE TABLE ${safeTableName} (
        id BIGSERIAL PRIMARY KEY,
        title TEXT NOT NULL
      );
    `,
  );
}

async function openCollectionSynonyms(page: DashboardPage, tableName: string): Promise<void> {
  await page.goto("/admin/");
  await waitForDashboard(page);

  const sidebar = page.getByRole("complementary");
  await expect(sidebar.getByText(tableName, { exact: true })).toBeVisible({ timeout: 10000 });
  await sidebar.getByText(tableName, { exact: true }).click();

  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: tableName, exact: true })).toBeVisible({
    timeout: 10000,
  });

  const synonymsTab = main.getByRole("button", { name: /^Synonyms$/ });
  await expect(synonymsTab).toBeVisible({ timeout: 10000 });
  await synonymsTab.click();

  await expect(
    main.getByRole("heading", { name: `Search synonyms for ${tableName}` }),
  ).toBeVisible({ timeout: 10000 });
}

function firstSynonymGroup(page: DashboardPage) {
  return page.getByRole("group", { name: "Synonym group 1" });
}

async function expectGroupTerms(
  group: ReturnType<typeof firstSynonymGroup>,
  expectedTerms: string[],
): Promise<void> {
  const actualTerms = await group
    .getByRole("textbox")
    .evaluateAll((nodes) => nodes.map((node) => (node as HTMLInputElement).value).sort());
  expect(actualTerms).toEqual([...expectedTerms].sort());
}

async function addAndSaveFirstGroup(
  page: DashboardPage,
  firstTerm: string,
  secondTerm: string,
): Promise<void> {
  await page.getByRole("button", { name: "Add group" }).click();
  const group = firstSynonymGroup(page);
  await group.getByRole("textbox").nth(0).fill(firstTerm);
  await group.getByRole("textbox").nth(1).fill(secondTerm);
  await page.getByRole("button", { name: "Save" }).click();
  await expect(page.getByRole("status")).toHaveText("Saved search synonyms.");
  await expectGroupTerms(group, [firstTerm, secondTerm]);
}

test.describe("Collection Synonyms Editor (Full E2E)", () => {
  const tablesToDropByTestID = new Map<string, string[]>();

  function registerTableForCleanup(testID: string, tableName: string) {
    const tables = tablesToDropByTestID.get(testID) ?? [];
    tables.push(tableName);
    tablesToDropByTestID.set(testID, tables);
  }

  test.afterEach(async ({ request, adminToken }, testInfo) => {
    const tablesToDropAfterTest = tablesToDropByTestID.get(testInfo.testId) ?? [];
    tablesToDropByTestID.delete(testInfo.testId);

    for (const tableName of tablesToDropAfterTest) {
      await dropTableIfExists(request, adminToken, tableName, "collection synonyms test table").catch(
        () => {},
      );
    }
  });

  test("renders a synonym group seeded through the admin contract", async ({
    page,
    request,
    adminToken,
  }, testInfo) => {
    const runID = buildParallelSafeRunID(testInfo);
    const tableName = `collection_synonyms_seed_${runID}`;
    registerTableForCleanup(testInfo.testId, tableName);

    await recreateSearchableCollection(request, adminToken, tableName);
    await replaceCollectionSearchSynonyms(request, adminToken, tableName, [
      { terms: ["scifi", "science fiction"] },
    ]);

    await openCollectionSynonyms(page, tableName);

    const group = firstSynonymGroup(page);
    await expect(group.getByRole("textbox")).toHaveCount(2);
    await expectGroupTerms(group, ["scifi", "science fiction"]);
  });

  test("saves a new synonym group with exactly two terms", async ({
    page,
    request,
    adminToken,
  }, testInfo) => {
    const runID = buildParallelSafeRunID(testInfo);
    const tableName = `collection_synonyms_create_${runID}`;
    registerTableForCleanup(testInfo.testId, tableName);

    await recreateSearchableCollection(request, adminToken, tableName);

    await openCollectionSynonyms(page, tableName);

    await expect(page.getByText("No synonym groups configured")).toBeVisible();
    await addAndSaveFirstGroup(page, "space opera", "galactic adventure");
  });

  test("rejects a single-term group before saving", async ({
    page,
    request,
    adminToken,
  }, testInfo) => {
    const runID = buildParallelSafeRunID(testInfo);
    const tableName = `collection_synonyms_validation_${runID}`;
    registerTableForCleanup(testInfo.testId, tableName);

    await recreateSearchableCollection(request, adminToken, tableName);

    await openCollectionSynonyms(page, tableName);

    await page.getByRole("button", { name: "Add group" }).click();
    const group = firstSynonymGroup(page);
    await group.getByRole("textbox").nth(0).fill("single term");
    await group.getByRole("textbox").nth(1).fill("");
    await page.getByRole("button", { name: "Save" }).click();

    await expect(page.getByText("Each synonym group needs at least two terms.")).toBeVisible();
    await expect(group.getByRole("textbox").nth(0)).toHaveValue("single term");
  });

  test("saved synonyms expand dashboard search results for the closeout collection", async ({
    page,
    request,
    adminToken,
  }) => {
    const matchingTitle = "Science fiction archive for closeout";
    const controlTitle = "Fantasy archive for closeout";

    await recreateSearchableCollection(request, adminToken, CLOSEOUT_COLLECTION);
    await seedRecord(request, adminToken, CLOSEOUT_COLLECTION, { title: matchingTitle });
    await seedRecord(request, adminToken, CLOSEOUT_COLLECTION, { title: controlTitle });

    await openCollectionSynonyms(page, CLOSEOUT_COLLECTION);
    await addAndSaveFirstGroup(page, "scifi", "science fiction");

    const sidebar = page.getByRole("complementary");
    await sidebar.getByText("Search", { exact: true }).click();
    await expect(page.getByRole("heading", { name: "Search" })).toBeVisible({ timeout: 10000 });

    await page.getByLabel("Collection").selectOption(CLOSEOUT_COLLECTION);
    await page.getByLabel("Search query").fill("scifi");
    await page.getByRole("main").getByRole("button", { name: /^Search$/i }).click();

    await expect(page.getByText(matchingTitle, { exact: true })).toBeVisible({ timeout: 10000 });
    await expect(page.getByText(controlTitle, { exact: true })).toBeHidden();
  });
});
