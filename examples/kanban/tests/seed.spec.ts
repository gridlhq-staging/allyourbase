import { test, expect } from "@playwright/test";
import { waitForAnonymousBoardShell, ownedBoardCount, ownedBoardId } from "./helpers";

/**
 * Stage 2: a fresh anonymous user (delivered by the anonymous-first entry
 * flow) is seeded exactly one starter board with starter columns and cards.
 *
 * Counts are scoped to the current user via `ownedBoardCount` — the demo's
 * board list shows every user's boards (boards_select USING (true)), so a
 * global DOM count would be polluted by parallel test workers.
 */
test.describe("Sample board seeding", () => {
  test("fresh anonymous user lands on one seeded board with starter columns", async ({
    page,
  }) => {
    await waitForAnonymousBoardShell(page);

    // The anonymous user owns exactly the one seeded board.
    await expect.poll(() => ownedBoardCount(page)).toBe(1);

    // Open the current user's seeded board by its test ID, then confirm its
    // three starter columns. Using the board-specific testid scopes the click
    // to the current user's board rather than any globally-visible one.
    const boardId = await ownedBoardId(page);
    expect(boardId).not.toBeNull();
    await page.getByTestId(`board-${boardId}`).click();
    const toDo = page.getByTestId("column-To Do");
    const inProgress = page.getByTestId("column-In Progress");
    const done = page.getByTestId("column-Done");
    await expect(toDo).toBeVisible();
    await expect(inProgress).toBeVisible();
    await expect(done).toBeVisible();

    // Stage 2 requires four starter cards, distributed across the columns.
    // Card text is scoped to each column container so the assertion targets
    // this user's seeded board, not any globally-visible "My First Board".
    await expect(toDo.getByText("Welcome to your board")).toBeVisible();
    await expect(toDo.getByText("Drag cards between columns")).toBeVisible();
    await expect(inProgress.getByText("Invite a teammate")).toBeVisible();
    await expect(done.getByText("Ship something")).toBeVisible();
  });

  test("page reload does not duplicate the seeded board", async ({ page }) => {
    await waitForAnonymousBoardShell(page);
    await expect.poll(() => ownedBoardCount(page)).toBe(1);

    // A full reload re-runs the anonymous-first flow and seed check in the
    // same context; idempotency must keep the owned-board count at one.
    await page.reload();
    await waitForAnonymousBoardShell(page);
    await expect.poll(() => ownedBoardCount(page)).toBe(1);
  });

  test("stale in-progress marker with missing starter cards is repaired on reload", async ({
    page,
  }) => {
    await waitForAnonymousBoardShell(page);
    await expect.poll(() => ownedBoardCount(page)).toBe(1);

    const boardId = await ownedBoardId(page);
    expect(boardId).not.toBeNull();

    // Simulate an interrupted seed run after columns exist but before cards
    // finish writing: keep marker set and delete one seeded starter card.
    await page.evaluate(async ({ targetBoardId }) => {
      const token = sessionStorage.getItem("ayb_token");
      if (!token) throw new Error("seed-repair test: no auth token in sessionStorage");
      const headers = { Authorization: `Bearer ${token}` };

      const meRes = await fetch("/api/auth/me", { headers });
      if (!meRes.ok) throw new Error(`seed-repair test: /api/auth/me failed (${meRes.status})`);
      const me = (await meRes.json()) as { id: string };

      localStorage.setItem("ayb_kanban_seed_in_progress", me.id);

      const columnsFilter = encodeURIComponent(`board_id='${targetBoardId}'`);
      const columnsRes = await fetch(
        `/api/collections/columns?filter=${columnsFilter}&perPage=100`,
        { headers },
      );
      if (!columnsRes.ok) {
        throw new Error(`seed-repair test: columns list failed (${columnsRes.status})`);
      }
      const columns = (await columnsRes.json()) as {
        items: { id: string; title: string }[];
      };
      const doneColumn = columns.items.find((col) => col.title === "Done");
      if (!doneColumn) {
        throw new Error("seed-repair test: missing seeded Done column");
      }

      const cardsFilter = encodeURIComponent(`column_id='${doneColumn.id}'`);
      const cardsRes = await fetch(
        `/api/collections/cards?filter=${cardsFilter}&perPage=100`,
        { headers },
      );
      if (!cardsRes.ok) {
        throw new Error(`seed-repair test: cards list failed (${cardsRes.status})`);
      }
      const cards = (await cardsRes.json()) as {
        items: { id: string; title: string }[];
      };
      const shipCard = cards.items.find((card) => card.title === "Ship something");
      if (!shipCard) {
        throw new Error("seed-repair test: missing expected starter card to delete");
      }

      const deleteRes = await fetch(`/api/collections/cards/${shipCard.id}`, {
        method: "DELETE",
        headers,
      });
      if (!deleteRes.ok) {
        throw new Error(`seed-repair test: delete card failed (${deleteRes.status})`);
      }
    }, { targetBoardId: boardId });

    await page.reload();
    await waitForAnonymousBoardShell(page);
    await expect.poll(() => ownedBoardCount(page)).toBe(1);

    const reloadedBoardId = await ownedBoardId(page);
    expect(reloadedBoardId).not.toBeNull();
    await page.getByTestId(`board-${reloadedBoardId}`).click();
    await expect(page.getByTestId("column-Done").getByText("Ship something")).toBeVisible();
  });

  test("repair does not duplicate the starter board when board deletion fails", async ({
    page,
  }) => {
    await waitForAnonymousBoardShell(page);
    await expect.poll(() => ownedBoardCount(page)).toBe(1);

    const boardId = await ownedBoardId(page);
    expect(boardId).not.toBeNull();

    await page.evaluate(async ({ targetBoardId }) => {
      const token = sessionStorage.getItem("ayb_token");
      if (!token) throw new Error("seed-repair delete-failure test: no auth token in sessionStorage");
      const headers = { Authorization: `Bearer ${token}` };

      const meRes = await fetch("/api/auth/me", { headers });
      if (!meRes.ok) {
        throw new Error(`seed-repair delete-failure test: /api/auth/me failed (${meRes.status})`);
      }
      const me = (await meRes.json()) as { id: string };

      localStorage.setItem("ayb_kanban_seed_in_progress", me.id);

      const columnsFilter = encodeURIComponent(`board_id='${targetBoardId}'`);
      const columnsRes = await fetch(
        `/api/collections/columns?filter=${columnsFilter}&perPage=100`,
        { headers },
      );
      if (!columnsRes.ok) {
        throw new Error(`seed-repair delete-failure test: columns list failed (${columnsRes.status})`);
      }
      const columns = (await columnsRes.json()) as {
        items: { id: string; title: string }[];
      };
      const doneColumn = columns.items.find((col) => col.title === "Done");
      if (!doneColumn) {
        throw new Error("seed-repair delete-failure test: missing seeded Done column");
      }

      const cardsFilter = encodeURIComponent(`column_id='${doneColumn.id}'`);
      const cardsRes = await fetch(
        `/api/collections/cards?filter=${cardsFilter}&perPage=100`,
        { headers },
      );
      if (!cardsRes.ok) {
        throw new Error(`seed-repair delete-failure test: cards list failed (${cardsRes.status})`);
      }
      const cards = (await cardsRes.json()) as {
        items: { id: string; title: string }[];
      };
      const shipCard = cards.items.find((card) => card.title === "Ship something");
      if (!shipCard) {
        throw new Error("seed-repair delete-failure test: missing expected starter card to delete");
      }

      const deleteRes = await fetch(`/api/collections/cards/${shipCard.id}`, {
        method: "DELETE",
        headers,
      });
      if (!deleteRes.ok) {
        throw new Error(`seed-repair delete-failure test: delete card failed (${deleteRes.status})`);
      }
    }, { targetBoardId: boardId });

    await page.route(`**/api/collections/boards/${boardId}`, async (route) => {
      if (route.request().method() === "DELETE") {
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "seeded delete failure for test" }),
        });
        return;
      }
      await route.continue();
    });

    await page.reload();
    await waitForAnonymousBoardShell(page);
    await expect.poll(() => ownedBoardCount(page)).toBe(1);
  });

  test("failed initial board create does not leave a stale in-progress marker", async ({
    page,
  }) => {
    // Fail the very first seed board-create so seeding throws before any
    // board exists for this user. Only POST is failed; the GET list call
    // the seed check issues first must still pass through.
    let boardCreateFailed = false;
    await page.route("**/api/collections/boards**", async (route) => {
      if (route.request().method() === "POST" && !boardCreateFailed) {
        boardCreateFailed = true;
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "seeded board-create failure for test" }),
        });
        return;
      }
      await route.continue();
    });

    await waitForAnonymousBoardShell(page);
    expect(boardCreateFailed).toBe(true);

    // The board create failed before any board existed, so the in-progress
    // marker must not persist. A stale marker would let a later legitimate
    // single "My First Board" with fewer than three columns be misread as
    // an interrupted seed run and deleted by the repair path.
    await expect
      .poll(() =>
        page.evaluate(() => localStorage.getItem("ayb_kanban_seed_in_progress")),
      )
      .toBeNull();
  });
});
