import { test, expect, execSQL, waitForDashboard } from "../fixtures";
import type { Locator, Page } from "@playwright/test";

/**
 * SMOKE TEST: Schema View
 *
 * Critical Path: Open seeded table → switch to Schema tab → verify structural schema details
 */

test.describe("Smoke: Schema View", () => {
  const cleanupSQL: string[] = [];

  async function openTableFromSidebar(page: Page, tableName: string): Promise<void> {
    const sidebar = page.locator("aside");
    const refreshButton = page.getByRole("button", { name: /refresh schema/i });
    const tableLink = sidebar.getByText(tableName, { exact: true });

    await expect(refreshButton).toBeVisible({ timeout: 5000 });
    await expect
      .poll(
        async () => {
          await refreshButton.click();
          return tableLink.isVisible();
        },
        { timeout: 15000 },
      )
      .toBe(true);

    await tableLink.click();
  }

  test.afterEach(async ({ request, adminToken }) => {
    for (const sql of cleanupSQL) {
      await execSQL(request, adminToken, sql).catch(() => {});
    }
    cleanupSQL.length = 0;
  });

  function columnRow(table: Locator, columnName: string) {
    return table.getByRole("row").filter({
      has: table.page().getByRole("cell", { name: columnName, exact: true }),
    });
  }

  test("seeded table schema renders columns, constraints, indexes, and comments", async ({
    page,
    request,
    adminToken,
  }) => {
    const runId = Date.now();
    const parentTableName = `smoke_schema_parent_${runId}`;
    const childTableName = `smoke_schema_child_${runId}`;
    const plainTableName = `smoke_schema_plain_${runId}`;
    const foreignKeyName = `smoke_schema_child_parent_fk_${runId}`;
    const indexName = `smoke_schema_child_code_idx_${runId}`;
    const childComment = `Schema smoke child table ${runId}`;

    cleanupSQL.push(
      `DROP TABLE IF EXISTS ${childTableName};`,
      `DROP TABLE IF EXISTS ${plainTableName};`,
      `DROP TABLE IF EXISTS ${parentTableName};`,
    );

    await execSQL(
      request,
      adminToken,
      `CREATE TABLE ${parentTableName} (
        id SERIAL PRIMARY KEY,
        name TEXT NOT NULL
      );

      CREATE TABLE ${childTableName} (
        id SERIAL PRIMARY KEY,
        parent_id INTEGER NOT NULL,
        code TEXT NOT NULL,
        notes TEXT,
        created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
        CONSTRAINT ${foreignKeyName}
          FOREIGN KEY (parent_id)
          REFERENCES ${parentTableName}(id)
          ON DELETE CASCADE
      );

      CREATE TABLE ${plainTableName} (
        id SERIAL PRIMARY KEY,
        label TEXT NOT NULL
      );

      CREATE UNIQUE INDEX ${indexName} ON ${childTableName} USING btree (code);

      COMMENT ON TABLE ${childTableName} IS '${childComment}';

      INSERT INTO ${parentTableName} (name) VALUES ('Schema Parent');
      INSERT INTO ${childTableName} (parent_id, code, notes)
      VALUES (1, 'schema-child', NULL);`,
    );

    await page.goto("/admin/");
    await waitForDashboard(page);

    await openTableFromSidebar(page, childTableName);
    await page.getByRole("button", { name: /^Schema$/i }).click();

    const main = page.locator("main");

    await expect(main.getByRole("heading", { name: "Columns" })).toBeVisible({
      timeout: 15_000,
    });
    await expect(main.getByRole("columnheader", { name: "Name" }).first()).toBeVisible();
    await expect(main.getByRole("columnheader", { name: "Type" }).first()).toBeVisible();
    await expect(main.getByRole("columnheader", { name: "Nullable" }).first()).toBeVisible();
    await expect(main.getByRole("columnheader", { name: "Default" }).first()).toBeVisible();

    const columnsTable = main.getByRole("table").first();
    await expect(columnRow(columnsTable, "id")).toContainText("integer");
    await expect(columnRow(columnsTable, "id")).toContainText("no");
    await expect(columnRow(columnsTable, "parent_id")).toContainText("integer");
    await expect(columnRow(columnsTable, "parent_id")).toContainText("no");
    await expect(columnRow(columnsTable, "code")).toContainText("text");
    await expect(columnRow(columnsTable, "code")).toContainText("no");
    await expect(columnRow(columnsTable, "notes")).toContainText("yes");
    await expect(columnRow(columnsTable, "notes")).toContainText("—");
    await expect(columnRow(columnsTable, "created_at")).toContainText(
      "timestamp",
    );
    await expect(columnRow(columnsTable, "created_at")).toContainText("now()");

    await expect(main.getByRole("heading", { name: "Foreign Keys" })).toBeVisible();
    await expect(main.getByText(foreignKeyName, { exact: true })).toBeVisible();
    await expect(main.getByText("parent_id", { exact: true }).nth(1)).toBeVisible();
    await expect(main.getByText(`public.${parentTableName}(id)`, { exact: true })).toBeVisible();
    await expect(main).toContainText("ON DELETE CASCADE");

    await expect(main.getByRole("heading", { name: "Indexes" })).toBeVisible();
    const indexesTable = main.getByRole("table").nth(1);
    await expect(indexesTable).toContainText(`${childTableName}_pkey`);
    await expect(indexesTable).toContainText(indexName);
    await expect(indexesTable.getByRole("row").filter({ hasText: indexName })).toContainText(
      "btree",
    );
    await expect(indexesTable.getByRole("row").filter({ hasText: indexName })).toContainText(
      "yes",
    );

    await expect(main.getByRole("heading", { name: "Relationships" })).toBeVisible();
    await expect(main.getByText("many-to-one", { exact: true })).toBeVisible();
    await expect(
      main.getByText(`${childTableName}(parent_id) → ${parentTableName}(id)`, { exact: true }),
    ).toBeVisible();

    await expect(main.getByRole("heading", { name: "Comment" })).toBeVisible();
    await expect(main.getByText(childComment, { exact: true })).toBeVisible();

    await openTableFromSidebar(page, plainTableName);
    await page.getByRole("button", { name: /^Schema$/i }).click();

    await expect(main.getByRole("heading", { name: "Columns" })).toBeVisible();
    await expect(main.getByRole("heading", { name: "Foreign Keys" })).toBeHidden();
    await expect(main.getByRole("heading", { name: "Relationships" })).toBeHidden();
    await expect(main.getByRole("heading", { name: "Comment" })).toBeHidden();
  });
});
