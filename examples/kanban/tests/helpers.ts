import { type Page, type Route, expect } from "@playwright/test";
import {
  installRealtimeReadinessProbe,
  waitForRealtimeTables,
} from "../../../tests/e2e/realtime_readiness";

let userCounter = 0;
let nameCounter = 0;
export const runId = Math.random().toString(36).slice(2, 8);

/** Generate a unique board/resource name to avoid collisions across parallel workers
 *  (boards_select USING (true) makes all boards visible to all users). */
export function uniqueName(base: string): string {
  return `${base} ${runId}-${++nameCounter}`;
}

/** Generate a unique test user email to avoid collisions between test runs. */
export function uniqueEmail(): string {
  return `test-${runId}-${Date.now()}-${++userCounter}@example.com`;
}

export const TEST_PASSWORD = "testpassword123";
const ANONYMOUS_BOOTSTRAP_OPTOUT_KEY = "ayb_anonymous_bootstrap_optout";
const SEED_IN_PROGRESS_KEY = "ayb_kanban_seed_in_progress";
type OwnedBoardProjection = "count" | "first-id";

type BlockedRequestGate = {
  wasBlocked: () => boolean;
  release: () => void;
};

type MockRecord = Record<string, unknown> & { id: string };

type MockKanbanApiOptions = {
  boards?: MockRecord[];
  columns?: MockRecord[];
  cards?: MockRecord[];
  attachments?: MockRecord[];
  persistCreates?: boolean;
};

type AuthenticatedSession = {
  token: string;
  refreshToken: string;
  email: string;
  userId: string;
};

type InstalledRoute = {
  uninstall: () => Promise<void>;
};

type RequestTrackingRoute = InstalledRoute & {
  waitForRequest: () => Promise<void>;
};

type FailingAttachmentDeleteRoute = InstalledRoute & {
  interceptedObjectName: () => string | null;
};

/** Demo account credentials. */
export const DEMO_ACCOUNTS = [
  { email: "alice@demo.test", password: "password123" },
  { email: "bob@demo.test", password: "password123" },
  { email: "charlie@demo.test", password: "password123" },
];

/** Ensure the login/register form is visible, signing out anonymous bootstrap when needed. */
export async function ensureAuthFormVisible(page: Page): Promise<void> {
  // Tests that explicitly exercise login/register should opt out of anonymous
  // bootstrap before navigation to avoid consuming anonymous rate-limit quota.
  await installRealtimeReadinessProbe(page);
  await page.addInitScript((optOutKey) => {
    localStorage.setItem(optOutKey, "1");
  }, ANONYMOUS_BOOTSTRAP_OPTOUT_KEY);
  await page.goto("/");

  const signInButton = page.getByRole("button", { name: "Sign In" });
  const signOutButton = page.getByText("Sign out");

  await expect
    .poll(async () => {
      if (await signInButton.isVisible()) {
        return "signin";
      }
      if (await signOutButton.isVisible()) {
        return "signout";
      }
      return "unknown";
    }, { timeout: 15000 })
    .not.toBe("unknown");

  if (await signOutButton.isVisible()) {
    await signOutButton.click();
  }

  await expect(signInButton).toBeVisible({ timeout: 10000 });
}

/** Wait for anonymous-first entry to land in the board shell, retrying across short auth rate-limit windows. */
export async function waitForAnonymousBoardShell(page: Page): Promise<void> {
  await page.goto("/");
  const deadline = Date.now() + 45000;
  const boardShell = page.getByText("Your Boards");
  const signInButton = page.getByRole("button", { name: "Sign In" });

  while (Date.now() < deadline) {
    const remaining = Math.max(deadline - Date.now(), 1);
    const state = await expect
      .poll(async () => {
        if (await boardShell.isVisible().catch(() => false)) {
          return "board";
        }
        if (await signInButton.isVisible().catch(() => false)) {
          return "signin";
        }
        return "pending";
      }, {
        timeout: Math.min(remaining, 3000),
        intervals: [100, 250, 500, 1000],
      })
      .not.toBe("pending")
      .then(async () => {
        if (await boardShell.isVisible().catch(() => false)) {
          return "board";
        }
        if (await signInButton.isVisible().catch(() => false)) {
          return "signin";
        }
        return "pending";
      })
      .catch(() => "pending");

    if (state === "board") {
      await expect(boardShell).toBeVisible({ timeout: 1000 });
      return;
    }
    if (state === "signin") {
      await page.reload();
    }
  }

  await expect(boardShell).toBeVisible({ timeout: 1000 });
}

/** Login with a demo account by clicking it to fill credentials, then signing in. */
export async function loginWithDemoAccount(
  page: Page,
  email: string = DEMO_ACCOUNTS[0].email,
): Promise<void> {
  await ensureAuthFormVisible(page);
  const acct = DEMO_ACCOUNTS.find((a) => a.email === email)!;
  await page.getByText(acct.email).click();
  await page.getByRole("button", { name: "Sign In" }).click();
  await expect(page.getByText("Your Boards")).toBeVisible({ timeout: 10000 });
}

/** Register a new user and return the email. */
export async function registerUser(page: Page): Promise<string> {
  const email = uniqueEmail();
  await ensureAuthFormVisible(page);

  // Switch to register mode
  await page.getByRole("button", { name: "Sign up" }).click();

  // Fill form
  await page.getByPlaceholder("you@example.com").fill(email);
  await page.getByPlaceholder("At least 8 characters").fill(TEST_PASSWORD);
  await page.getByRole("button", { name: "Create Account" }).click();

  // Wait for board list to load (auth succeeded).
  // First registration can be slow due to managed Postgres cold-start + bcrypt.
  await expect(page.getByText("Your Boards")).toBeVisible({ timeout: 15000 });

  return email;
}

/** Login with existing credentials. */
export async function loginUser(page: Page, email: string): Promise<void> {
  await ensureAuthFormVisible(page);
  await page.getByPlaceholder("you@example.com").fill(email);
  await page.getByPlaceholder("At least 8 characters").fill(TEST_PASSWORD);
  await page.getByRole("button", { name: "Sign In" }).click();
  await expect(page.getByText("Your Boards")).toBeVisible({ timeout: 10000 });
}

/** Create a board and return the board title. */
export async function createBoard(
  page: Page,
  title: string,
): Promise<void> {
  const titleInput = page.getByPlaceholder("New board name...");
  const createButton = page.getByRole("button", { name: "Create" });

  await titleInput.fill(title);
  await expect(titleInput).toHaveValue(title);
  await expect(createButton).toBeEnabled();
  const [createResponse] = await Promise.all([
    page.waitForResponse((response) => {
      return (
        response.request().method() === "POST" &&
        new URL(response.url()).pathname.endsWith("/api/collections/boards")
      );
    }),
    createButton.click(),
  ]);
  expect(createResponse.ok()).toBe(true);
  await expect(titleInput).toHaveValue("");
  // Use .first() — collaborative model means other users' boards are visible,
  // so duplicate titles from other workers may exist.
  await expect(page.getByText(title).first()).toBeVisible();
}

/** Navigate into a board. */
export async function openBoard(page: Page, title: string): Promise<void> {
  // Use .first() — boards sorted by -created_at, so most recent is first.
  await page.getByText(title).first().click();
  await expect(
    page.getByRole("heading", { name: title }),
  ).toBeVisible({ timeout: 5000 });
  await waitForRealtimeTables(page, ["cards", "columns"]);
}

/** Add a column to the current board. */
export async function addColumn(
  page: Page,
  title: string,
): Promise<void> {
  await page.getByPlaceholder("+ Add column...").fill(title);
  await page.getByRole("button", { name: "Add Column" }).click();
  await expect(page.getByText(title)).toBeVisible();
}

/**
 * Query the current user's owned boards through the API instead of the DOM.
 *
 * The kanban demo intentionally exposes every board in the shared list via RLS,
 * so DOM-level board counts are polluted by other workers. Keeping the auth +
 * filter logic in one helper avoids drift between count/id callers.
 */
async function readOwnedBoardProjection(
  page: Page,
  projection: OwnedBoardProjection,
): Promise<number | string | null> {
  return page.evaluate(async (mode) => {
    const quoteCollectionFilterLiteral = (value: string) =>
      `'${value.replace(/\\/g, "\\\\").replace(/'/g, "\\'")}'`;
    const token = sessionStorage.getItem("ayb_token");
    if (!token) {
      throw new Error("readOwnedBoardProjection: no auth token in sessionStorage");
    }
    const headers = { Authorization: `Bearer ${token}` };

    const meRes = await fetch("/api/auth/me", { headers });
    if (!meRes.ok) {
      throw new Error(`readOwnedBoardProjection: /api/auth/me failed (${meRes.status})`);
    }
    const me = (await meRes.json()) as { id: string };

    const filter = encodeURIComponent(`user_id=${quoteCollectionFilterLiteral(me.id)}`);
    const boardsRes = await fetch(
      `/api/collections/boards?filter=${filter}&perPage=100`,
      { headers },
    );
    if (!boardsRes.ok) {
      throw new Error(`readOwnedBoardProjection: boards list failed (${boardsRes.status})`);
    }
    const data = (await boardsRes.json()) as { items: { id: string; user_id: string }[] };
    const ownedBoards = data.items.filter((board) => board.user_id === me.id);

    if (mode === "count") {
      return ownedBoards.length;
    }
    return ownedBoards[0]?.id ?? null;
  }, projection);
}

/**
 * Count boards owned by the currently-authenticated user.
 *
 * `boards_select USING (true)` (schema.sql) makes every board globally
 * visible, so the demo's board list — and any DOM-level count — includes
 * other test workers' boards. Idempotency assertions must key on user-owned
 * boards, so this queries the API directly with a `user_id` filter and
 * re-checks ownership client-side. Lives in helpers.ts because spec files are
 * lint-banned from raw API calls; helpers are exempt.
 */
export async function ownedBoardCount(page: Page): Promise<number> {
  const ownedBoardCount = await readOwnedBoardProjection(page, "count");
  if (typeof ownedBoardCount !== "number") {
    throw new Error(`ownedBoardCount: expected numeric board count, got ${typeof ownedBoardCount}`);
  }
  return ownedBoardCount;
}

/** Get the board ID of the first board owned by the current user. */
export async function ownedBoardId(page: Page): Promise<string | null> {
  const ownedBoardId = await readOwnedBoardProjection(page, "first-id");
  if (ownedBoardId !== null && typeof ownedBoardId !== "string") {
    throw new Error(`ownedBoardId: expected string or null, got ${typeof ownedBoardId}`);
  }
  return ownedBoardId;
}

/** Return the authenticated storage bucket route status for reachability checks. */
export async function storageBucketStatus(page: Page): Promise<number> {
  return page.evaluate(async () => {
    const token = sessionStorage.getItem("ayb_token");
    if (!token) {
      throw new Error("storageBucketStatus: no auth token in sessionStorage");
    }
    const res = await fetch("/api/storage/card-attachments", {
      headers: { Authorization: `Bearer ${token}` },
    });
    return res.status;
  });
}

/** Delete a storage object through the authenticated demo session, tolerating prior cleanup. */
export async function deleteStorageObject(
  page: Page,
  bucket: string,
  objectName: string,
): Promise<void> {
  await page.evaluate(async ({ bucketName, objectPath }) => {
    const token = sessionStorage.getItem("ayb_token");
    if (!token) {
      throw new Error("deleteStorageObject: no auth token in sessionStorage");
    }
    const encodedBucket = encodeURIComponent(bucketName);
    const encodedObjectPath = objectPath.split("/").map(encodeURIComponent).join("/");
    const res = await fetch(`/api/storage/${encodedBucket}/${encodedObjectPath}`, {
      method: "DELETE",
      headers: { Authorization: `Bearer ${token}` },
    });
    if (!res.ok && res.status !== 404) {
      throw new Error(`deleteStorageObject: delete failed (${res.status})`);
    }
  }, {
    bucketName: bucket,
    objectPath: objectName,
  });
}

/** Fail storage-object DELETE once so tests can exercise metadata-first attachment cleanup. */
export async function installFailingAttachmentDeleteRoute(
  page: Page,
): Promise<FailingAttachmentDeleteRoute> {
  let objectName: string | null = null;
  const routePattern = "**/api/storage/card-attachments/**";
  const handler = async (route: Route) => {
    const request = route.request();
    if (request.method() !== "DELETE") {
      await route.continue();
      return;
    }
    const path = new URL(request.url()).pathname;
    const prefix = "/api/storage/card-attachments/";
    if (!path.startsWith(prefix)) {
      await route.continue();
      return;
    }
    objectName = decodeURIComponent(path.slice(prefix.length));
    await route.fulfill({
      status: 500,
      contentType: "application/json",
      body: JSON.stringify({ message: "storage cleanup failed" }),
    });
  };
  await page.route(routePattern, handler);
  return {
    interceptedObjectName: () => objectName,
    uninstall: () => page.unroute(routePattern, handler),
  };
}

/** Install an authenticated in-memory Kanban API for deterministic state tests. */
export async function installMockKanbanApi(
  page: Page,
  options: MockKanbanApiOptions = {},
): Promise<void> {
  const records = {
    boards: [...(options.boards ?? [])],
    columns: [...(options.columns ?? [])],
    cards: [...(options.cards ?? [])],
    attachments: [...(options.attachments ?? [])],
  };

  await installAuthenticatedSession(page, {
    token: "state-test-token",
    refreshToken: "state-test-refresh",
    email: "state-test@example.com",
    userId: "state-test-user",
  });

  await page.route("**/api/storage/card-attachments", async (route, request) => {
    if (request.method() === "POST") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          bucket: "card-attachments",
          name: "state-upload.txt",
          size: 18,
          content_type: "text/plain",
          created_at: new Date().toISOString(),
        }),
      });
      return;
    }
    await route.fallback();
  });

  await page.route("**/api/collections/**", async (route, request) => {
    const url = new URL(request.url());
    const [, collection, id] = url.pathname.match(/\/api\/collections\/([^/]+)\/?([^/]*)/) ?? [];
    if (!isMockedKanbanCollection(collection)) {
      await route.fallback();
      return;
    }

    const method = request.method();
    if (method === "GET") {
      await fulfillList(route, records[collection]);
      return;
    }
    if (method === "POST") {
      const created = {
        id: `${collection}-${records[collection].length + 1}`,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        ...(request.postDataJSON() as Record<string, unknown>),
      };
      if (options.persistCreates ?? true) {
        records[collection].push(created);
      }
      await fulfillJSON(route, 200, created);
      return;
    }
    if (method === "PATCH" && id) {
      const body = request.postDataJSON() as Record<string, unknown>;
      const existingIndex = records[collection].findIndex((record) => record.id === id);
      const updated = {
        ...records[collection][existingIndex],
        ...body,
        id,
        updated_at: new Date().toISOString(),
      };
      if (existingIndex >= 0) {
        records[collection][existingIndex] = updated;
      }
      await fulfillJSON(route, 200, updated);
      return;
    }
    if (method === "DELETE" && id) {
      records[collection] = records[collection].filter((record) => record.id !== id);
      await route.fulfill({ status: 204 });
      return;
    }

    await route.fallback();
  });
}

/** Fail matching collection requests so specs can assert visible error states. */
export async function installFailingCollectionRoute(
  page: Page,
  collection: "boards" | "columns" | "cards" | "attachments",
  method: "GET" | "POST" | "PATCH" | "DELETE",
  message: string,
): Promise<RequestTrackingRoute> {
  const routePattern = `**/api/collections/${collection}**`;
  let intercepted = false;
  let resolveIntercepted: () => void;
  const interceptedRequest = new Promise<void>((resolve) => {
    resolveIntercepted = resolve;
  });
  const handler = async (route: Route) => {
    const request = route.request();
    if (request.method() !== method) {
      await route.fallback();
      return;
    }
    intercepted = true;
    resolveIntercepted();
    await fulfillJSON(route, 500, { message });
  };
  await page.route(routePattern, handler);
  return {
    uninstall: () => page.unroute(routePattern, handler),
    waitForRequest: async () => {
      if (intercepted) return;
      await interceptedRequest;
    },
  };
}

/** Fail storage upload requests so specs can assert attachment upload errors. */
export async function installFailingStorageUploadRoute(
  page: Page,
  message: string,
): Promise<void> {
  await page.route("**/api/storage/card-attachments", async (route, request) => {
    if (request.method() !== "POST") {
      await route.fallback();
      return;
    }
    await fulfillJSON(route, 500, { message });
  });
}

/**
 * Delay matching collection requests until the test releases them.
 *
 * `afterRelease` selects how a held request continues once released:
 * `"fallback"` hands off to a later mock route (mocked-backend specs), while
 * `"continue"` lets the request reach the network (real-backend specs).
 *
 * `holdSubsequent` widens the gate from a single occurrence to "occurrence and
 * every later matching request", which loading-state assertions need under
 * React StrictMode: a mounted component's load effect fires twice in dev, so
 * holding only the first request lets the second resolve and clear the loading
 * UI before it can be asserted. Holding all pending loads keeps the component's
 * `loading` flag true until the gate is released.
 */
export async function blockCollectionRequest(
  page: Page,
  collection: "boards" | "columns" | "cards" | "attachments",
  method: "GET" | "POST" | "PATCH" | "DELETE",
  { occurrence = 1, afterRelease = "fallback", holdSubsequent = false }: BlockCollectionRequestOptions = {},
): Promise<BlockedRequestGate> {
  let seen = 0;
  let blocked = false;
  let released = false;
  let releaseHeldRequests = () => {};
  const heldUntilReleased = new Promise<void>((resolve) => {
    releaseHeldRequests = resolve;
  });

  await page.route(`**/api/collections/${collection}**`, async (route, request) => {
    if (request.method() === method) {
      seen += 1;
      const shouldHold = holdSubsequent ? seen >= occurrence : seen === occurrence;
      if (shouldHold && !released) {
        blocked = true;
        await heldUntilReleased;
      }
    }
    if (afterRelease === "continue") {
      await route.continue();
    } else {
      await route.fallback();
    }
  });

  return {
    wasBlocked: () => blocked,
    release: () => {
      if (!blocked) {
        throw new Error("expected collection request to be blocked before release");
      }
      released = true;
      releaseHeldRequests();
    },
  };
}

interface BlockCollectionRequestOptions {
  occurrence?: number;
  afterRelease?: "continue" | "fallback";
  holdSubsequent?: boolean;
}

function isMockedKanbanCollection(
  collection: string | undefined,
): collection is "boards" | "columns" | "cards" | "attachments" {
  return (
    collection === "boards" ||
    collection === "columns" ||
    collection === "cards" ||
    collection === "attachments"
  );
}

async function fulfillList(route: Route, items: MockRecord[]): Promise<void> {
  await fulfillJSON(route, 200, {
    items,
    page: 1,
    perPage: 100,
    totalItems: items.length,
    totalPages: items.length > 0 ? 1 : 0,
  });
}

async function fulfillJSON(route: Route, status: number, value: unknown): Promise<void> {
  await route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(value),
  });
}

async function installAuthenticatedSession(
  page: Page,
  session: AuthenticatedSession,
): Promise<void> {
  await page.addInitScript(({ optOutKey, token, refreshToken, email }) => {
    sessionStorage.setItem("ayb_token", token);
    sessionStorage.setItem("ayb_refresh_token", refreshToken);
    localStorage.setItem("ayb_email", email);
    localStorage.setItem(optOutKey, "1");
  }, {
    optOutKey: ANONYMOUS_BOOTSTRAP_OPTOUT_KEY,
    token: session.token,
    refreshToken: session.refreshToken,
    email: session.email,
  });

  await page.route("**/api/auth/me", async (route) => {
    await fulfillJSON(route, 200, {
      id: session.userId,
      email: session.email,
    });
  });
}

/** Simulate a partial starter-board seed by keeping the marker and deleting the Done starter card. */
export async function simulateInterruptedSeedMissingDoneCard(
  page: Page,
  boardId: string,
): Promise<void> {
  await page.evaluate(async ({ seedMarkerKey, targetBoardId }) => {
    const quoteCollectionFilterLiteral = (value: string) =>
      `'${value.replace(/\\/g, "\\\\").replace(/'/g, "\\'")}'`;
    const token = sessionStorage.getItem("ayb_token");
    if (!token) {
      throw new Error("simulateInterruptedSeedMissingDoneCard: no auth token in sessionStorage");
    }
    const headers = { Authorization: `Bearer ${token}` };

    const meRes = await fetch("/api/auth/me", { headers });
    if (!meRes.ok) {
      throw new Error(
        `simulateInterruptedSeedMissingDoneCard: /api/auth/me failed (${meRes.status})`,
      );
    }
    const me = (await meRes.json()) as { id: string };

    localStorage.setItem(seedMarkerKey, me.id);

    const columnsFilter = encodeURIComponent(`board_id=${quoteCollectionFilterLiteral(targetBoardId)}`);
    const columnsRes = await fetch(
      `/api/collections/columns?filter=${columnsFilter}&perPage=100`,
      { headers },
    );
    if (!columnsRes.ok) {
      throw new Error(
        `simulateInterruptedSeedMissingDoneCard: columns list failed (${columnsRes.status})`,
      );
    }
    const columns = (await columnsRes.json()) as {
      items: { id: string; title: string }[];
    };
    const doneColumn = columns.items.find((col) => col.title === "Done");
    if (!doneColumn) {
      throw new Error("simulateInterruptedSeedMissingDoneCard: missing seeded Done column");
    }

    const cardsFilter = encodeURIComponent(`column_id=${quoteCollectionFilterLiteral(doneColumn.id)}`);
    const cardsRes = await fetch(
      `/api/collections/cards?filter=${cardsFilter}&perPage=100`,
      { headers },
    );
    if (!cardsRes.ok) {
      throw new Error(
        `simulateInterruptedSeedMissingDoneCard: cards list failed (${cardsRes.status})`,
      );
    }
    const cards = (await cardsRes.json()) as {
      items: { id: string; title: string }[];
    };
    const shipCard = cards.items.find((card) => card.title === "Ship something");
    if (!shipCard) {
      throw new Error(
        "simulateInterruptedSeedMissingDoneCard: missing expected starter card to delete",
      );
    }

    const deleteRes = await fetch(`/api/collections/cards/${shipCard.id}`, {
      method: "DELETE",
      headers,
    });
    if (!deleteRes.ok) {
      throw new Error(
        `simulateInterruptedSeedMissingDoneCard: delete card failed (${deleteRes.status})`,
      );
    }
  }, {
    seedMarkerKey: SEED_IN_PROGRESS_KEY,
    targetBoardId: boardId,
  });
}

/** Read the sample-board seed marker from localStorage. */
export async function seedInProgressMarker(page: Page): Promise<string | null> {
  return page.evaluate((seedMarkerKey) => {
    return localStorage.getItem(seedMarkerKey);
  }, SEED_IN_PROGRESS_KEY);
}

/** Install a mocked authenticated board API and return a reader for the columns filter it receives. */
export async function installEscapedBoardApiMock(
  page: Page,
  boardId: string,
  boardTitle: string,
): Promise<() => string | null> {
  let capturedColumnsFilter: string | null = null;

  await installAuthenticatedSession(page, {
    token: "test-token",
    refreshToken: "test-refresh",
    email: "security-test@example.com",
    userId: "user-security-test",
  });
  await page.route("**/api/collections/boards**", async (route, request) => {
    if (request.method() !== "GET") {
      await route.continue();
      return;
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        items: [
          {
            id: boardId,
            title: boardTitle,
            created_at: new Date().toISOString(),
            user_id: "test-user",
          },
        ],
        page: 1,
        perPage: 20,
        totalItems: 1,
        totalPages: 1,
      }),
    });
  });
  await page.route("**/api/collections/columns**", async (route, request) => {
    capturedColumnsFilter = new URL(request.url()).searchParams.get("filter");
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({
        items: [],
        page: 1,
        perPage: 100,
        totalItems: 0,
        totalPages: 0,
      }),
    });
  });

  return () => capturedColumnsFilter;
}

/**
 * Delay the first column-create request until the test releases it.
 *
 * Real-backend specs use this to hold a local create in flight; it delegates to
 * the shared block-until-released seam with `continue` so the held request still
 * reaches the network once released.
 */
export async function blockFirstColumnCreate(page: Page): Promise<BlockedRequestGate> {
  return blockCollectionRequest(page, "columns", "POST", { afterRelease: "continue" });
}

/** Add a card to a column. */
export async function addCard(
  page: Page,
  columnTitle: string,
  cardTitle: string,
): Promise<void> {
  // Scope to column via data-testid added to the column container
  const column = page.getByTestId(`column-${columnTitle}`);
  await column.getByText("+ Add a card").click();
  await column.getByPlaceholder("Card title...").fill(cardTitle);
  await column.getByRole("button", { name: "Add", exact: true }).click();
  await expect(page.getByText(cardTitle)).toBeVisible();
}
