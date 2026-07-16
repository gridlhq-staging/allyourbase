import {
  adminURL,
  buildParallelSafeRunID,
  dropTableIfExists,
  execSQL,
  expect,
  expectChunkRequestCount,
  failNextMatchingChunk,
  observeJavaScriptChunks,
  sqlLiteral,
  test,
  waitForDashboard,
} from "../fixtures";
import type { APIRequestContext, Page, TestInfo } from "@playwright/test";

const USERS_CHUNK_PATTERN = /Users-[^/]+\.js(?:[?#]|$)/;
const ENTRY_CHUNK_PATTERN = /index-[^/]+\.js(?:[?#]|$)/;

async function openLazyScreen(
  page: Parameters<typeof waitForDashboard>[0],
  screen: string,
): Promise<void> {
  await page.getByRole("complementary").getByRole("button", { name: screen, exact: true }).click();
  await expect(page.getByRole("main").getByRole("heading", { name: screen, exact: true })).toBeVisible();
}

async function seedLandingProbeTable(
  request: APIRequestContext,
  adminToken: string,
  testInfo: TestInfo,
): Promise<{ tableName: string; rowName: string }> {
  const runID = buildParallelSafeRunID(testInfo);
  const tableName = `__lazy_refresh_${runID}`;
  const rowName = `Lazy refresh landed ${runID}`;

  await dropTableIfExists(request, adminToken, tableName, "lazy refresh probe table");
  await execSQL(
    request,
    adminToken,
    `CREATE TABLE ${tableName} (id SERIAL PRIMARY KEY, name TEXT NOT NULL);`,
  );
  await execSQL(
    request,
    adminToken,
    `INSERT INTO ${tableName} (name) VALUES ('${sqlLiteral(rowName)}');`,
  );

  return { tableName, rowName };
}

async function seedUsersRouteProbe(
  request: APIRequestContext,
  adminToken: string,
  testInfo: TestInfo,
): Promise<{ fallbackTableName: string; usersRowName: string }> {
  const runID = buildParallelSafeRunID(testInfo);
  const fallbackTableName = `__lazy_first_${runID}`;
  const usersRowName = `Lazy users route landed ${runID}`;

  await dropTableIfExists(request, adminToken, fallbackTableName, "lazy refresh fallback table");
  await dropTableIfExists(request, adminToken, "users", "lazy refresh users table");
  await execSQL(
    request,
    adminToken,
    `CREATE TABLE ${fallbackTableName} (id SERIAL PRIMARY KEY, name TEXT NOT NULL);
     INSERT INTO ${fallbackTableName} (name) VALUES ('fallback should not land');
     CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT NOT NULL);
     INSERT INTO users (name) VALUES ('${sqlLiteral(usersRowName)}');`,
  );

  return { fallbackTableName, usersRowName };
}

async function expectLandingProbeContent(
  page: Page,
  probe: { tableName: string; rowName: string },
): Promise<void> {
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: probe.tableName, exact: true })).toBeVisible();
  await expect(main.getByRole("cell", { name: probe.rowName, exact: true })).toBeVisible();
}

async function expectUsersScreenContent(page: Page): Promise<void> {
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: "Users", exact: true })).toBeVisible();
  await expect(main.getByText("Manage registered user accounts", { exact: true })).toBeVisible();
}

async function expectUsersTableContent(
  page: Page,
  probe: { fallbackTableName: string; usersRowName: string },
): Promise<void> {
  const main = page.getByRole("main");
  await expect(main.getByRole("heading", { name: "users", exact: true })).toBeVisible();
  await expect(main.getByRole("cell", { name: probe.usersRowName, exact: true })).toBeVisible();
  await expect(main.getByRole("heading", { name: probe.fallbackTableName, exact: true })).toHaveCount(0);
}

test.describe("Lazy admin screen chunks", () => {
  test.describe.configure({ mode: "serial" });

  test("loads a registry screen chunk on demand and fetches it again after retry", async ({ page }) => {
    const chunks = observeJavaScriptChunks(page);

    try {
      await page.goto(adminURL("/"));
      await waitForDashboard(page);

      await expectChunkRequestCount(chunks.requestedChunkURLs, USERS_CHUNK_PATTERN, 0);

      await openLazyScreen(page, "Users");
      await expectChunkRequestCount(chunks.requestedChunkURLs, USERS_CHUNK_PATTERN, 1);

      await failNextMatchingChunk(page, USERS_CHUNK_PATTERN);
      await page.reload();
      await waitForDashboard(page);

      await expect(page.getByRole("heading", { name: "Something went wrong" })).toBeVisible();
      await page.getByRole("button", { name: "Retry" }).click();
      await waitForDashboard(page);

      await expectUsersScreenContent(page);
      await expectChunkRequestCount(chunks.requestedChunkURLs, USERS_CHUNK_PATTERN, 3);
    } finally {
      chunks.dispose();
    }
  });

  test("loads entry and Users chunk after hard refresh at /", async ({ page, request, adminToken }, testInfo) => {
    const chunks = observeJavaScriptChunks(page);
    const probe = await seedLandingProbeTable(request, adminToken, testInfo);

    try {
      await page.goto(adminURL("/"));
      await waitForDashboard(page);
      await expect(page.getByRole("complementary")).toBeVisible();
      await expectLandingProbeContent(page, probe);
      await expectChunkRequestCount(chunks.requestedChunkURLs, ENTRY_CHUNK_PATTERN, 1);
      await expectChunkRequestCount(chunks.requestedChunkURLs, USERS_CHUNK_PATTERN, 0);

      await openLazyScreen(page, "Users");
      await expectChunkRequestCount(chunks.requestedChunkURLs, USERS_CHUNK_PATTERN, 1);
    } finally {
      chunks.dispose();
      await dropTableIfExists(request, adminToken, probe.tableName, "lazy refresh probe table");
    }
  });

  test("loads entry and Storage chunk after hard refresh at /screens/users", async ({ page }) => {
    const chunks = observeJavaScriptChunks(page);

    try {
      await page.goto(adminURL("/screens/users"));
      await waitForDashboard(page);
      await expect(page.getByRole("complementary")).toBeVisible();
      await expectUsersScreenContent(page);
      await expectChunkRequestCount(chunks.requestedChunkURLs, ENTRY_CHUNK_PATTERN, 1);
      await expectChunkRequestCount(chunks.requestedChunkURLs, /StorageBrowser-[^/]+\.js(?:[?#]|$)/, 0);

      await openLazyScreen(page, "Storage");
      await expectChunkRequestCount(chunks.requestedChunkURLs, /StorageBrowser-[^/]+\.js(?:[?#]|$)/, 1);
    } finally {
      chunks.dispose();
    }
  });

  test("loads entry and API Keys chunk after hard refresh at /tables/public/users", async ({ page, request, adminToken }, testInfo) => {
    const chunks = observeJavaScriptChunks(page);
    const probe = await seedUsersRouteProbe(request, adminToken, testInfo);

    try {
      await page.goto(adminURL("/tables/public/users"));
      await waitForDashboard(page);
      await expect(page.getByRole("complementary")).toBeVisible();
      await expectUsersTableContent(page, probe);
      await expectChunkRequestCount(chunks.requestedChunkURLs, ENTRY_CHUNK_PATTERN, 1);
      await expectChunkRequestCount(chunks.requestedChunkURLs, /ApiKeys-[^/]+\.js(?:[?#]|$)/, 0);

      await openLazyScreen(page, "API Keys");
      await expectChunkRequestCount(chunks.requestedChunkURLs, /ApiKeys-[^/]+\.js(?:[?#]|$)/, 1);
    } finally {
      chunks.dispose();
      await dropTableIfExists(request, adminToken, probe.fallbackTableName, "lazy refresh fallback table");
      await dropTableIfExists(request, adminToken, "users", "lazy refresh users table");
    }
  });
});
