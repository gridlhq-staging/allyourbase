import AxeBuilder from "@axe-core/playwright";
import { test, expect, type Page } from "@playwright/test";
import { Buffer } from "node:buffer";
import {
  addCard,
  addColumn,
  blockCollectionRequest,
  createBoard,
  ensureAuthFormVisible,
  installFailingAttachmentDeleteRoute,
  installFailingCollectionRoute,
  installFailingStorageUploadRoute,
  installMockKanbanApi,
  registerUser,
} from "./helpers";
import { dragCardToColumn } from "./state_arrangements";

const existingBoard = {
  id: "a11y-existing-board",
  title: "Existing Board",
  user_id: "state-test-user",
  created_at: "2026-07-21T12:00:00.000Z",
  updated_at: "2026-07-21T12:00:00.000Z",
};

// Copied from ui/browser-tests-unmocked/smoke/accessibility.spec.ts::assertAccessible.
// Keep axe tags and critical/serious assertion semantics in sync; this demo omits the UI-only .cm-editor exclusion.
async function assertAccessible(page: Page, pageName: string): Promise<void> {
  const results = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    .analyze();

  for (const violation of results.violations) {
    console.log(
      `[a11y] ${pageName}: ${violation.id} impact=${violation.impact ?? "unknown"} nodes=${violation.nodes.length}`,
    );
  }

  const critical = results.violations.filter(
    (violation) => violation.impact === "critical" || violation.impact === "serious",
  );
  const minor = results.violations.filter(
    (violation) => violation.impact === "moderate" || violation.impact === "minor",
  );

  if (minor.length > 0) {
    console.log(
      `[a11y] ${pageName}: ${minor.length} moderate/minor issue(s):`,
      minor.map((violation) => `${violation.id}: ${violation.help} (${violation.nodes.length} node(s))`),
    );
  }

  expect(
    critical,
    `${pageName}: ${critical.length} critical/serious a11y violation(s): ${critical
      .map((violation) => `${violation.id}: ${violation.help}`)
      .join("; ")}`,
  ).toHaveLength(0);
}

async function openMockBoard(page: Page, boardName: string): Promise<void> {
  await installMockKanbanApi(page, { boards: [existingBoard] });
  await page.goto("/");
  await createBoard(page, boardName);
  await page.getByRole("button", { name: new RegExp(`Open board ${boardName}`) }).click();
  await expect(page.getByRole("heading", { name: boardName })).toBeVisible();
}

async function openMockBoardWithCard(page: Page): Promise<void> {
  await openMockBoard(page, "A11y Board");
  await addColumn(page, "To Do");
  await addColumn(page, "Done");
  await addCard(page, "To Do", "A11y Card");
}

async function openCardModal(page: Page): Promise<void> {
  await openMockBoardWithCard(page);
  await page.getByText("A11y Card").click();
  await expect(page.getByRole("dialog")).toBeVisible();
}

test.describe("Kanban accessibility states", () => {
  test("a11y: auth sign-in", async ({ page }) => {
    await ensureAuthFormVisible(page);
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();
    await assertAccessible(page, "auth sign-in");
  });

  test("a11y: auth register", async ({ page }) => {
    await ensureAuthFormVisible(page);
    await page.getByRole("button", { name: "Sign up" }).click();
    await expect(page.getByRole("button", { name: "Create Account" })).toBeVisible();
    await assertAccessible(page, "auth register");
  });

  test("a11y: auth error", async ({ page }) => {
    await ensureAuthFormVisible(page);
    await page.getByPlaceholder("you@example.com").fill("missing@example.com");
    await page.getByPlaceholder("At least 8 characters").fill("wrong-password");
    await page.getByRole("button", { name: "Sign In" }).click();
    await expect(page.getByRole("alert")).toBeVisible();
    await assertAccessible(page, "auth error");
  });

  test("a11y: first-paint loading", async ({ page }) => {
    await installMockKanbanApi(page);
    const gate = await blockCollectionRequest(page, "boards", "GET", { occurrence: 1 });
    await page.goto("/");
    await expect.poll(() => gate.wasBlocked()).toBe(true);
    await expect(page.getByText("Loading...", { exact: true })).toBeVisible();
    await assertAccessible(page, "first-paint loading");
    gate.release();
  });

  test("a11y: board-list empty", async ({ page }) => {
    await installMockKanbanApi(page, { persistCreates: false });
    await page.goto("/");
    await expect(page.getByText("No boards yet")).toBeVisible();
    await assertAccessible(page, "board-list empty");
  });

  test("a11y: board-list populated", async ({ page }) => {
    await installMockKanbanApi(page);
    await page.goto("/");
    await createBoard(page, "A11y Board");
    await expect(page.getByRole("button", { name: /Open board A11y Board/ })).toBeVisible();
    await assertAccessible(page, "board-list populated");
  });

  test("a11y: board-list loading", async ({ page }) => {
    await installMockKanbanApi(page);
    const gate = await blockCollectionRequest(page, "boards", "GET", {
      occurrence: 2,
      holdSubsequent: true,
    });
    await page.goto("/");
    await expect.poll(() => gate.wasBlocked()).toBe(true);
    await expect(page.getByText("Loading boards...")).toBeVisible();
    await assertAccessible(page, "board-list loading");
    gate.release();
  });

  test("a11y: board-list load error", async ({ page }) => {
    await installMockKanbanApi(page);
    await installFailingCollectionRoute(page, "boards", "GET", "Board list unavailable");
    await page.goto("/");
    await expect(page.getByRole("alert")).toContainText("Board list unavailable");
    await assertAccessible(page, "board-list load error");
  });

  test("a11y: board create error", async ({ page }) => {
    await installMockKanbanApi(page);
    await page.goto("/");
    await installFailingCollectionRoute(page, "boards", "POST", "Board create failed");
    await page.getByPlaceholder("New board name...").fill("Broken Create Board");
    await page.getByRole("button", { name: "Create" }).click();
    await expect(page.getByRole("alert")).toContainText("Board create failed");
    await assertAccessible(page, "board create error");
  });

  test("a11y: board delete error", async ({ page }) => {
    await installMockKanbanApi(page);
    await page.goto("/");
    await createBoard(page, "A11y Board");
    await installFailingCollectionRoute(page, "boards", "DELETE", "Board delete failed");
    page.on("dialog", (dialog) => dialog.accept());
    await page.getByRole("button", { name: /Open board A11y Board/ }).hover();
    await page.getByRole("button", { name: "Delete board A11y Board" }).click();
    await expect(page.getByRole("alert")).toContainText("Board delete failed");
    await assertAccessible(page, "board delete error");
  });

  test("a11y: board empty", async ({ page }) => {
    await openMockBoard(page, "A11y Board");
    await expect(page.getByRole("heading", { name: "A11y Board" })).toBeVisible();
    await assertAccessible(page, "board empty");
  });

  test("a11y: board populated", async ({ page }) => {
    await openMockBoardWithCard(page);
    await expect(page.getByText("A11y Card")).toBeVisible();
    await assertAccessible(page, "board populated");
  });

  test("a11y: board loading", async ({ page }) => {
    await installMockKanbanApi(page);
    const gate = await blockCollectionRequest(page, "columns", "GET", { holdSubsequent: true });
    await page.goto("/");
    await createBoard(page, "A11y Board");
    await page.getByRole("button", { name: /Open board A11y Board/ }).click();
    await expect.poll(() => gate.wasBlocked()).toBe(true);
    await expect(page.getByText("Loading board...")).toBeVisible();
    await assertAccessible(page, "board loading");
    gate.release();
  });

  test("a11y: board load error", async ({ page }) => {
    await installMockKanbanApi(page);
    await page.goto("/");
    await createBoard(page, "A11y Board");
    await installFailingCollectionRoute(page, "columns", "GET", "Board columns failed");
    await page.getByRole("button", { name: /Open board A11y Board/ }).click();
    await expect(page.getByRole("alert")).toContainText("Board columns failed");
    await assertAccessible(page, "board load error");
  });

  test("a11y: add-column form", async ({ page }) => {
    await openMockBoard(page, "A11y Board");
    await page.getByPlaceholder("+ Add column...").fill("Review");
    await expect(page.getByPlaceholder("+ Add column...")).toHaveValue("Review");
    await assertAccessible(page, "add-column form");
  });

  test("a11y: add-card form", async ({ page }) => {
    await openMockBoard(page, "A11y Board");
    await addColumn(page, "To Do");
    await page.getByTestId("column-To Do").getByText("+ Add a card").click();
    await expect(page.getByPlaceholder("Card title...")).toBeVisible();
    await assertAccessible(page, "add-card form");
  });

  test("a11y: column create error", async ({ page }) => {
    await openMockBoard(page, "A11y Board");
    await installFailingCollectionRoute(page, "columns", "POST", "Column create failed");
    await page.getByPlaceholder("+ Add column...").fill("Broken Column");
    await page.getByRole("button", { name: "Add Column" }).click();
    await expect(page.getByRole("alert")).toContainText("Column create failed");
    await assertAccessible(page, "column create error");
  });

  test("a11y: column delete error", async ({ page }) => {
    await openMockBoard(page, "A11y Board");
    await addColumn(page, "To Do");
    await installFailingCollectionRoute(page, "columns", "DELETE", "Column delete failed");
    page.on("dialog", (dialog) => dialog.accept());
    await page.getByRole("button", { name: "Delete column To Do" }).click();
    await expect(page.getByText("Column delete failed")).toBeVisible();
    await assertAccessible(page, "column delete error");
  });

  test("a11y: card create error", async ({ page }) => {
    await openMockBoard(page, "A11y Board");
    await addColumn(page, "To Do");
    await installFailingCollectionRoute(page, "cards", "POST", "Card create failed");
    const column = page.getByTestId("column-To Do");
    await column.getByText("+ Add a card").click();
    await column.getByPlaceholder("Card title...").fill("Broken Card");
    await column.getByRole("button", { name: "Add", exact: true }).click();
    await expect(column.getByRole("alert")).toContainText("Card create failed");
    await assertAccessible(page, "card create error");
  });

  test("a11y: card move error", async ({ page }) => {
    await openMockBoardWithCard(page);
    await installFailingCollectionRoute(page, "cards", "PATCH", "Card move failed");
    await dragCardToColumn(
      page,
      page.getByText("A11y Card"),
      page.locator("[data-rfd-droppable-id]").nth(1),
    );
    await expect(page.getByRole("alert")).toContainText("Card move failed");
    await assertAccessible(page, "card move error");
  });

  test("a11y: card modal default", async ({ page }) => {
    await openCardModal(page);
    await expect(page.getByRole("dialog").getByText("No attachments")).toBeVisible();
    await assertAccessible(page, "card modal default");
  });

  test("a11y: card modal validation", async ({ page }) => {
    await openCardModal(page);
    const modal = page.getByRole("dialog");
    await modal.getByLabel("Title").fill("");
    await expect(modal.getByRole("button", { name: "Save" })).toBeDisabled();
    await assertAccessible(page, "card modal validation");
  });

  test("a11y: card save error", async ({ page }) => {
    await openCardModal(page);
    await installFailingCollectionRoute(page, "cards", "PATCH", "Card save failed");
    const modal = page.getByRole("dialog");
    await modal.getByLabel("Title").fill("Broken Save");
    await modal.getByLabel("Description").fill("Save error description");
    await modal.getByRole("button", { name: "Save" }).click();
    await expect(modal.getByRole("alert")).toContainText("Card save failed");
    await assertAccessible(page, "card save error");
  });

  test("a11y: card delete error", async ({ page }) => {
    await openCardModal(page);
    await installFailingCollectionRoute(page, "cards", "DELETE", "Card delete failed");
    page.on("dialog", (dialog) => dialog.accept());
    const modal = page.getByRole("dialog");
    await modal.getByText("Delete card").click();
    await expect(modal.getByRole("alert")).toContainText("Card delete failed");
    await assertAccessible(page, "card delete error");
  });

  test("a11y: attachments loading", async ({ page }) => {
    await openMockBoardWithCard(page);
    const gate = await blockCollectionRequest(page, "attachments", "GET", { holdSubsequent: true });
    await page.getByText("A11y Card").click();
    const modal = page.getByRole("dialog");
    await expect.poll(() => gate.wasBlocked()).toBe(true);
    await expect(modal.getByText("Loading attachments...")).toBeVisible();
    await assertAccessible(page, "attachments loading");
    gate.release();
  });

  test("a11y: attachments empty", async ({ page }) => {
    await openCardModal(page);
    await expect(page.getByRole("dialog").getByText("No attachments")).toBeVisible();
    await assertAccessible(page, "attachments empty");
  });

  test("a11y: attachments populated", async ({ page }) => {
    await openCardModal(page);
    const modal = page.getByRole("dialog");
    await modal.getByLabel("Attach file").setInputFiles({
      name: "a11y-upload.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("a11y upload proof", "utf8"),
    });
    await expect(modal.getByText("a11y-upload.txt")).toBeVisible();
    await assertAccessible(page, "attachments populated");
  });

  test("a11y: attachments load error", async ({ page }) => {
    await openMockBoardWithCard(page);
    await installFailingCollectionRoute(page, "attachments", "GET", "Attachment load failed");
    await page.getByText("A11y Card").click();
    await expect(page.getByRole("dialog").getByRole("alert")).toContainText("Attachment load failed");
    await assertAccessible(page, "attachments load error");
  });

  test("a11y: attachments upload error", async ({ page }) => {
    await openCardModal(page);
    await installFailingStorageUploadRoute(page, "Attachment upload failed");
    const modal = page.getByRole("dialog");
    await modal.getByLabel("Attach file").setInputFiles({
      name: "broken-upload.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("broken upload proof", "utf8"),
    });
    await expect(modal.getByRole("alert")).toContainText("Attachment upload failed");
    await assertAccessible(page, "attachments upload error");
  });

  test("a11y: attachments delete error", async ({ page }) => {
    await openCardModal(page);
    const modal = page.getByRole("dialog");
    await modal.getByLabel("Attach file").setInputFiles({
      name: "delete-upload.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("delete upload proof", "utf8"),
    });
    await expect(modal.getByText("delete-upload.txt")).toBeVisible();
    await installFailingAttachmentDeleteRoute(page);
    await modal.getByRole("button", { name: "Delete delete-upload.txt" }).click();
    await expect(modal.getByRole("alert")).toContainText("file cleanup failed");
    await assertAccessible(page, "attachments delete error");
  });
});
