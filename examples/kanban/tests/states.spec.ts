import { test, expect } from "@playwright/test";
import { Buffer } from "node:buffer";
import {
  blockCollectionRequest,
  installFailingCollectionRoute,
  installFailingStorageUploadRoute,
  installMockKanbanApi,
} from "./helpers";
import { dragCardToColumn } from "./state_arrangements";

const mockBoard = {
  id: "state-board-1",
  title: "State Board",
  user_id: "state-test-user",
  created_at: "2026-07-21T12:00:00.000Z",
  updated_at: "2026-07-21T12:00:00.000Z",
};

const mockColumn = {
  id: "state-column-1",
  board_id: mockBoard.id,
  title: "To Do",
  position: 0,
  created_at: "2026-07-21T12:01:00.000Z",
};

const mockDoneColumn = {
  ...mockColumn,
  id: "state-column-2",
  title: "Done",
  position: 1,
};

const mockCard = {
  id: "state-card-1",
  column_id: mockColumn.id,
  title: "State Card",
  description: "",
  position: 0,
  created_at: "2026-07-21T12:02:00.000Z",
  updated_at: "2026-07-21T12:02:00.000Z",
};

test.describe("Kanban visible states", () => {
  test("shows first-paint loading while the seed board check is pending", async ({ page }) => {
    await installMockKanbanApi(page, { boards: [mockBoard] });
    // The seed's owned-board check is the first `boards` GET; it fires only
    // after auth has resolved, so blocking it isolates the App's first-paint
    // `Loading...` (`!seedChecked`) frame from the identical auth-bootstrap
    // frame. Prove the request is pending before asserting the copy.
    const seedCheckGate = await blockCollectionRequest(page, "boards", "GET", { occurrence: 1 });

    await page.goto("/");
    await expect.poll(() => seedCheckGate.wasBlocked()).toBe(true);
    await expect(page.getByText("Loading...", { exact: true })).toBeVisible();

    seedCheckGate.release();
    await expect(page.getByRole("button", { name: /Open board State Board/ })).toBeVisible();
  });

  test("shows board-list loading while the board list request is pending", async ({ page }) => {
    await installMockKanbanApi(page, { boards: [mockBoard] });
    // Let the seed check pass (first GET) and block the BoardList's own load
    // (second GET onward), so the pending request that owns `Loading boards...`
    // is distinct from the first-paint gate above and from any auth frame.
    // `holdSubsequent` holds StrictMode's duplicate load too, so `loading`
    // cannot clear early.
    const boardListGate = await blockCollectionRequest(page, "boards", "GET", {
      occurrence: 2,
      holdSubsequent: true,
    });

    await page.goto("/");
    await expect.poll(() => boardListGate.wasBlocked()).toBe(true);
    await expect(page.getByText("Loading boards...")).toBeVisible();

    boardListGate.release();
    await expect(page.getByRole("button", { name: /Open board State Board/ })).toBeVisible();
  });

  test("shows empty board-list guidance for an authenticated zero-board shell", async ({ page }) => {
    await installMockKanbanApi(page, { persistCreates: false });

    await page.goto("/");

    await expect(page.getByText("No boards yet")).toBeVisible();
    await expect(page.getByText("Create your first board above")).toBeVisible();
  });

  test("shows board-list load, create, and delete failures", async ({ page }) => {
    await installMockKanbanApi(page, { boards: [mockBoard] });
    await installFailingCollectionRoute(page, "boards", "GET", "Board list unavailable");

    await page.goto("/");
    await expect(page.getByRole("alert")).toContainText("Board list unavailable");

    await installMockKanbanApi(page, { boards: [mockBoard] });
    await page.reload();
    await expect(page.getByRole("button", { name: /Open board State Board/ })).toBeVisible();

    await installFailingCollectionRoute(page, "boards", "POST", "Board create failed");
    await page.getByPlaceholder("New board name...").fill("Broken Create Board");
    await page.getByRole("button", { name: "Create" }).click();
    await expect(page.getByRole("alert")).toContainText("Board create failed");

    await installFailingCollectionRoute(page, "boards", "DELETE", "Board delete failed");
    page.on("dialog", (dialog) => dialog.accept());
    await page.getByRole("button", { name: /Open board State Board/ }).hover();
    await page.getByRole("button", { name: "Delete board State Board" }).click();
    await expect(page.getByRole("alert")).toContainText("Board delete failed");
  });

  test("shows board loading and board load failure", async ({ page }) => {
    await installMockKanbanApi(page, { boards: [mockBoard], columns: [mockColumn] });
    // Hold every board-columns load (StrictMode fires the effect twice) so the
    // `Loading board...` frame stays up until the gate is released.
    const gate = await blockCollectionRequest(page, "columns", "GET", { holdSubsequent: true });

    await page.goto("/");
    await page.getByRole("button", { name: /Open board State Board/ }).click();
    await expect.poll(() => gate.wasBlocked()).toBe(true);
    await expect(page.getByText("Loading board...")).toBeVisible();
    gate.release();
    await expect(page.getByTestId("column-To Do")).toBeVisible();

    await installFailingCollectionRoute(page, "columns", "GET", "Board columns failed");
    await page.getByRole("button", { name: "Back to boards" }).click();
    await page.getByRole("button", { name: /Open board State Board/ }).click();
    await expect(page.getByRole("alert")).toContainText("Board columns failed");
  });

  test("keeps later board mutation failures visible after a load failure", async ({ page }) => {
    await installMockKanbanApi(page, { boards: [mockBoard] });
    await installFailingCollectionRoute(page, "columns", "GET", "Board columns failed");

    await page.goto("/");
    await page.getByRole("button", { name: /Open board State Board/ }).click();
    await expect(page.getByRole("alert")).toContainText("Board columns failed");

    const failingColumnCreate = await installFailingCollectionRoute(
      page,
      "columns",
      "POST",
      "Column create failed",
    );
    await page.getByPlaceholder("+ Add column...").fill("Broken Column");
    await Promise.all([
      failingColumnCreate.waitForRequest(),
      page.getByRole("button", { name: "Add Column" }).click(),
    ]);
    await expect(
      page.getByRole("alert").filter({ hasText: "Column create failed" }),
    ).toBeVisible();
  });

  test("shows column and card mutation failures", async ({ page }) => {
    await installMockKanbanApi(page, {
      boards: [mockBoard],
      columns: [mockColumn],
      cards: [mockCard],
    });
    await page.goto("/");
    await page.getByRole("button", { name: /Open board State Board/ }).click();

    await installFailingCollectionRoute(page, "columns", "POST", "Column create failed");
    await page.getByPlaceholder("+ Add column...").fill("Broken Column");
    await page.getByRole("button", { name: "Add Column" }).click();
    await expect(page.getByRole("alert")).toContainText("Column create failed");

    await installFailingCollectionRoute(page, "columns", "DELETE", "Column delete failed");
    page.on("dialog", (dialog) => dialog.accept());
    await page.getByRole("button", { name: "Delete column To Do" }).click();
    await expect(page.getByText("Column delete failed")).toBeVisible();

    await installFailingCollectionRoute(page, "cards", "POST", "Card create failed");
    const column = page.getByTestId("column-To Do");
    await column.getByText("+ Add a card").click();
    await column.getByPlaceholder("Card title...").fill("Broken Card");
    await column.getByRole("button", { name: "Add", exact: true }).click();
    await expect(column.getByRole("alert")).toContainText("Card create failed");

    await installFailingCollectionRoute(page, "cards", "PATCH", "Card save failed");
    await page.getByText("State Card").click();
    const modal = page.getByRole("dialog");
    await modal.getByLabel("Title").fill("Broken Save");
    await modal.getByRole("button", { name: "Save" }).click();
    await expect(modal.getByRole("alert")).toContainText("Card save failed");

    await installFailingCollectionRoute(page, "cards", "DELETE", "Card delete failed");
    await modal.getByText("Delete card").click();
    await expect(modal.getByRole("alert")).toContainText("Card delete failed");
  });

  test("clears a failed card-move alert after a successful retry", async ({ page }) => {
    await installMockKanbanApi(page, {
      boards: [mockBoard],
      columns: [mockColumn, mockDoneColumn],
      cards: [mockCard],
    });
    await page.goto("/");
    await page.getByRole("button", { name: /Open board State Board/ }).click();

    const dragStateCardToDone = async () => {
      await dragCardToColumn(
        page,
        page.getByText("State Card"),
        page.locator("[data-rfd-droppable-id]").nth(1),
      );
    };

    const failedMove = await installFailingCollectionRoute(
      page,
      "cards",
      "PATCH",
      "Card move failed",
    );
    await dragStateCardToDone();
    await expect(page.getByRole("alert")).toContainText("Card move failed");
    await expect(page.getByTestId("column-To Do").getByTestId("card-count")).toHaveText("1");
    await expect(page.getByTestId("column-Done").getByTestId("card-count")).toHaveText("0");

    await failedMove.uninstall();
    await dragStateCardToDone();
    await expect(page.getByTestId("column-To Do").getByTestId("card-count")).toHaveText("0");
    await expect(page.getByTestId("column-Done").getByTestId("card-count")).toHaveText("1");
    await expect(page.getByText("Card move failed")).toBeHidden();
  });

  test("shows attachment loading, load failure, and upload failure", async ({ page }) => {
    await installMockKanbanApi(page, {
      boards: [mockBoard],
      columns: [mockColumn],
      cards: [mockCard],
    });
    // Hold every attachment load (StrictMode fires the effect twice) so the
    // `Loading attachments...` frame stays up until the gate is released.
    const gate = await blockCollectionRequest(page, "attachments", "GET", { holdSubsequent: true });

    await page.goto("/");
    await page.getByRole("button", { name: /Open board State Board/ }).click();
    await page.getByText("State Card").click();
    const modal = page.getByRole("dialog");
    await expect.poll(() => gate.wasBlocked()).toBe(true);
    await expect(modal.getByText("Loading attachments...")).toBeVisible();
    gate.release();
    await expect(modal.getByText("No attachments")).toBeVisible();

    await modal.getByRole("button", { name: "Cancel", exact: true }).click();
    await installFailingCollectionRoute(page, "attachments", "GET", "Attachment load failed");
    await page.getByText("State Card").click();
    await expect(page.getByRole("dialog").getByRole("alert")).toContainText(
      "Attachment load failed",
    );

    await modal.getByRole("button", { name: "Cancel", exact: true }).click();
    await installFailingStorageUploadRoute(page, "Attachment upload failed");
    await page.getByText("State Card").click();
    await page.getByRole("dialog").getByLabel("Attach file").setInputFiles({
      name: "state-upload.txt",
      mimeType: "text/plain",
      buffer: Buffer.from("state upload proof", "utf8"),
    });
    await expect(page.getByRole("dialog").getByRole("alert")).toContainText(
      "Attachment upload failed",
    );
  });
});
