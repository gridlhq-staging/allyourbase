import { type Page, expect } from "@playwright/test";

let userCounter = 0;
const runId = Math.random().toString(36).slice(2, 8);

/** Generate a unique test user email to avoid collisions between test runs. */
export function uniqueEmail(): string {
  return `test-${runId}-${Date.now()}-${++userCounter}@example.com`;
}

/** Generate a unique name (for libraries, etc.) to avoid collisions within a run. */
export function uniqueName(base: string): string {
  return `${base} ${runId}-${++userCounter}`;
}

export const TEST_PASSWORD = "testpassword123";

/** Register a new owner and return the email. Ends on the dashboard. */
export async function registerUser(page: Page): Promise<string> {
  const email = uniqueEmail();
  await page.goto("/signup");

  await page.getByPlaceholder("Email").fill(email);
  await page.getByPlaceholder("Password").fill(TEST_PASSWORD);
  await page.getByRole("button", { name: "Create Account" }).click();

  // Wait for dashboard to load (auth succeeded).
  // First registration can be slow due to embedded Postgres cold-start + bcrypt.
  // Use heading role to avoid matching both the NavBar link and the section heading.
  await expect(page.getByRole("heading", { name: "My Libraries" })).toBeVisible({ timeout: 30000 });

  return email;
}

/** Login with existing credentials. Ends on the dashboard. */
export async function loginUser(page: Page, email: string): Promise<void> {
  await page.goto("/login");
  await page.getByPlaceholder("Email").fill(email);
  await page.getByPlaceholder("Password").fill(TEST_PASSWORD);
  await page.getByRole("button", { name: "Sign In" }).click();
  await expect(page.getByRole("heading", { name: "My Libraries" })).toBeVisible({ timeout: 5000 });
}

/** Create a library from the dashboard. */
export async function createLibrary(
  page: Page,
  name: string,
): Promise<void> {
  await page.getByRole("button", { name: "+ New Library" }).click();
  await page.getByPlaceholder(/Power Tools/).fill(name);
  await page.getByRole("button", { name: "Create Library" }).click();
  // Wait for the create form to close (indicates successful creation).
  // When creation succeeds, the form is unmounted by Dashboard.
  await expect(page.getByRole("button", { name: "Create Library" })).not.toBeVisible({ timeout: 10000 });
}

/** Navigate into a library detail page from the dashboard. */
export async function openLibrary(page: Page, name: string): Promise<void> {
  // Use .first() since public libraries from other test runs may share the same name
  await page.getByRole("link", { name }).first().click();
  await expect(
    page.getByRole("heading", { name }),
  ).toBeVisible({ timeout: 10000 });
}

/** Get the share slug from the library detail page. Must be on library detail page. */
export async function getShareSlug(page: Page): Promise<string> {
  // The library detail page shows "Share link: {origin}/l/{slug}" as a <Link>
  const shareLink = page.locator('a[href*="/l/"]');
  const href = await shareLink.getAttribute("href");
  return href!.replace("/l/", "");
}

/** Add an item to the currently open library. */
export async function addItem(
  page: Page,
  itemName: string,
  description?: string,
): Promise<void> {
  await page.getByRole("link", { name: "+ Add Item" }).click();
  await page.getByPlaceholder("What is this item?").fill(itemName);
  if (description) {
    await page.getByPlaceholder("Any details about this item").fill(description);
  }
  await page.getByRole("button", { name: "Add Item" }).click();
  // Redirects back to library detail
  await expect(page.getByText(itemName)).toBeVisible({ timeout: 5000 });
}
