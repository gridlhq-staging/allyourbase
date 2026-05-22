import { type Page, expect } from "@playwright/test";

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

  while (Date.now() < deadline) {
    if (await page.getByText("Your Boards").isVisible().catch(() => false)) {
      await expect(page.getByText("Your Boards")).toBeVisible({ timeout: 1000 });
      return;
    }
    if (await page.getByRole("button", { name: "Sign In" }).isVisible().catch(() => false)) {
      await page.reload();
    } else {
      await page.waitForTimeout(1000);
    }
  }

  await expect(page.getByText("Your Boards")).toBeVisible({ timeout: 1000 });
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
  await page.getByPlaceholder("New board name...").fill(title);
  await page.getByRole("button", { name: "Create" }).click();
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
  return page.evaluate(async () => {
    const quoteFilterLiteral = (value: string) =>
      `'${value.replace(/\\/g, "\\\\").replace(/'/g, "\\'")}'`;
    const token = sessionStorage.getItem("ayb_token");
    if (!token) throw new Error("ownedBoardCount: no auth token in sessionStorage");
    const headers = { Authorization: `Bearer ${token}` };

    const meRes = await fetch("/api/auth/me", { headers });
    if (!meRes.ok) throw new Error(`ownedBoardCount: /api/auth/me failed (${meRes.status})`);
    const me = (await meRes.json()) as { id: string };

    const filter = encodeURIComponent(`user_id=${quoteFilterLiteral(me.id)}`);
    const boardsRes = await fetch(
      `/api/collections/boards?filter=${filter}&perPage=100`,
      { headers },
    );
    if (!boardsRes.ok) {
      throw new Error(`ownedBoardCount: boards list failed (${boardsRes.status})`);
    }
    const data = (await boardsRes.json()) as { items: { user_id: string }[] };
    return data.items.filter((b) => b.user_id === me.id).length;
  });
}

/** Get the board ID of the first board owned by the current user. */
export async function ownedBoardId(page: Page): Promise<string | null> {
  return page.evaluate(async () => {
    const quoteFilterLiteral = (value: string) =>
      `'${value.replace(/\\/g, "\\\\").replace(/'/g, "\\'")}'`;
    const token = sessionStorage.getItem("ayb_token");
    if (!token) throw new Error("ownedBoardId: no auth token in sessionStorage");
    const headers = { Authorization: `Bearer ${token}` };

    const meRes = await fetch("/api/auth/me", { headers });
    if (!meRes.ok) throw new Error(`ownedBoardId: /api/auth/me failed (${meRes.status})`);
    const me = (await meRes.json()) as { id: string };

    const filter = encodeURIComponent(`user_id=${quoteFilterLiteral(me.id)}`);
    const boardsRes = await fetch(
      `/api/collections/boards?filter=${filter}&perPage=100`,
      { headers },
    );
    if (!boardsRes.ok) {
      throw new Error(`ownedBoardId: boards list failed (${boardsRes.status})`);
    }
    const data = (await boardsRes.json()) as { items: { id: string; user_id: string }[] };
    const owned = data.items.find((b) => b.user_id === me.id);
    return owned ? owned.id : null;
  });
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
