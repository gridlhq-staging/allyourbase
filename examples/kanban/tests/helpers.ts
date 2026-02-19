import { type Page, expect } from "@playwright/test";

let userCounter = 0;
const runId = Math.random().toString(36).slice(2, 8);

/** Generate a unique test user email to avoid collisions between test runs. */
export function uniqueEmail(): string {
  return `test-${runId}-${Date.now()}-${++userCounter}@example.com`;
}

export const TEST_PASSWORD = "testpassword123";

/** Register a new user and return the email. */
export async function registerUser(page: Page): Promise<string> {
  const email = uniqueEmail();
  await page.goto("/");

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
  await page.goto("/");
  await page.getByPlaceholder("you@example.com").fill(email);
  await page.getByPlaceholder("At least 8 characters").fill(TEST_PASSWORD);
  await page.getByRole("button", { name: "Sign In" }).click();
  await expect(page.getByText("Your Boards")).toBeVisible({ timeout: 5000 });
}

/** Create a board and return the board title. */
export async function createBoard(
  page: Page,
  title: string,
): Promise<void> {
  await page.getByPlaceholder("New board name...").fill(title);
  await page.getByRole("button", { name: "Create" }).click();
  await expect(page.getByText(title)).toBeVisible();
}

/** Navigate into a board. */
export async function openBoard(page: Page, title: string): Promise<void> {
  await page.getByText(title).click();
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
  await column.getByRole("button", { name: "Add" }).click();
  await expect(page.getByText(cardTitle)).toBeVisible();
}
