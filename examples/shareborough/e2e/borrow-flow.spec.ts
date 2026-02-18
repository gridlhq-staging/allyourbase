import { test, expect } from "@playwright/test";
import { registerUser, createLibrary, openLibrary, addItem, uniqueName } from "./helpers";

test.describe("Borrow Flow", () => {
  // These tests set up an owner with a library and item,
  // then exercise the full borrow lifecycle.
  // Each test uses unique item and borrower names to avoid data pollution.

  async function setupOwnerWithItem(page: import("@playwright/test").Page) {
    const libName = uniqueName("Lending");
    const itemName = uniqueName("Sander");
    await registerUser(page);
    await createLibrary(page, libName);
    await openLibrary(page, libName);
    await addItem(page, itemName, "Great for woodworking");

    // Extract the slug from the share link on the library detail page
    const shareLinkEl = page.locator("a").filter({ hasText: /\/l\// });
    const href = await shareLinkEl.getAttribute("href");
    return { slug: href!.replace("/l/", ""), itemName };
  }

  /** Submit a borrow request from a public page and return to confirmation. */
  async function submitBorrowRequest(
    page: import("@playwright/test").Page,
    slug: string,
    itemName: string,
    borrowerName: string,
    borrowerPhone: string,
    message?: string,
  ) {
    const publicPage = await page.context().newPage();
    await publicPage.goto(`/l/${slug}`);
    await publicPage.getByText(itemName).click();
    await expect(publicPage.getByRole("button", { name: "Borrow This" })).toBeVisible({ timeout: 5000 });
    await publicPage.getByRole("button", { name: "Borrow This" }).click();
    await publicPage.getByPlaceholder("Your name").fill(borrowerName);
    await publicPage.getByPlaceholder("Phone number").fill(borrowerPhone);
    if (message) {
      await publicPage.getByPlaceholder(/Message to the owner/).fill(message);
    }
    await publicPage.getByRole("button", { name: "Send Request" }).click();
    await expect(publicPage.getByText("Request Sent!")).toBeVisible({ timeout: 10000 });
    await publicPage.close();
  }

  test("public library is accessible without auth", async ({ page }) => {
    const { slug, itemName } = await setupOwnerWithItem(page);

    const publicPage = await page.context().newPage();
    await publicPage.goto(`/l/${slug}`);
    await expect(publicPage.getByText(itemName)).toBeVisible({ timeout: 5000 });
    await publicPage.close();
  });

  test("public library shows items grid", async ({ page }) => {
    const { slug, itemName } = await setupOwnerWithItem(page);

    const publicPage = await page.context().newPage();
    await publicPage.goto(`/l/${slug}`);
    await expect(publicPage.getByText(itemName)).toBeVisible({ timeout: 5000 });
    await expect(publicPage.getByText(/available/i).first()).toBeVisible();
    await publicPage.close();
  });

  test("can open item detail from public library", async ({ page }) => {
    const { slug, itemName } = await setupOwnerWithItem(page);

    const publicPage = await page.context().newPage();
    await publicPage.goto(`/l/${slug}`);
    await publicPage.getByText(itemName).click();

    await expect(publicPage.getByRole("heading", { name: itemName })).toBeVisible({ timeout: 5000 });
    await expect(publicPage.getByText("Great for woodworking")).toBeVisible();
    await expect(publicPage.getByRole("button", { name: "Borrow This" })).toBeVisible();
    await publicPage.close();
  });

  test("can submit a borrow request", async ({ page }) => {
    const { slug, itemName } = await setupOwnerWithItem(page);
    const borrower = uniqueName("Bob");
    await submitBorrowRequest(page, slug, itemName, borrower, "555-123-4567", "Would love to use this!");
  });

  test("owner can approve a borrow request", async ({ page }) => {
    const { slug, itemName } = await setupOwnerWithItem(page);
    const borrower = uniqueName("Alice");
    await submitBorrowRequest(page, slug, itemName, borrower, "555-999-0001");

    // Owner goes to dashboard and approves the specific request
    await page.goto("/dashboard");
    await expect(page.getByText("Pending Requests")).toBeVisible({ timeout: 5000 });
    const requestCard = page.locator(".card").filter({ hasText: borrower }).first();
    await expect(requestCard).toBeVisible();
    await requestCard.getByRole("button", { name: "Approve" }).click();

    // After approval, a loan should appear
    await expect(page.getByText("Currently Borrowed")).toBeVisible({ timeout: 5000 });
  });

  test("owner can decline a borrow request", async ({ page }) => {
    const { slug, itemName } = await setupOwnerWithItem(page);
    const borrower = uniqueName("Charlie");
    await submitBorrowRequest(page, slug, itemName, borrower, "555-999-0002");

    // Owner declines the specific request
    await page.goto("/dashboard");
    await expect(page.getByText("Pending Requests")).toBeVisible({ timeout: 5000 });
    const requestCard = page.locator(".card").filter({ hasText: borrower }).first();
    await requestCard.getByRole("button", { name: "Decline" }).click();

    // Confirm in the ConfirmDialog modal if present
    const declineDialog = page.getByRole("dialog").getByRole("button", { name: "Decline" });
    if (await declineDialog.isVisible({ timeout: 2000 }).catch(() => false)) {
      await declineDialog.click();
    }

    // Wait for the decline action to complete — the request card should disappear
    await expect(requestCard).not.toBeVisible({ timeout: 10000 });
  });

  test("full lifecycle: borrow → approve → return", async ({ page }) => {
    const { slug, itemName } = await setupOwnerWithItem(page);
    const borrower = uniqueName("Dave");
    await submitBorrowRequest(page, slug, itemName, borrower, "555-999-0003");

    // Owner approves the specific request
    await page.goto("/dashboard");
    await expect(page.getByText("Pending Requests")).toBeVisible({ timeout: 5000 });
    const requestCard = page.locator(".card").filter({ hasText: borrower }).first();
    await requestCard.getByRole("button", { name: "Approve" }).click();
    await expect(page.getByText("Currently Borrowed")).toBeVisible({ timeout: 5000 });

    // Owner marks returned — scope to the loan card
    const loanCard = page.locator(".card").filter({ hasText: itemName }).filter({ hasText: borrower }).first();
    await loanCard.getByRole("button", { name: "Mark Returned" }).click();

    // Confirm in dialog
    await page.getByRole("dialog").getByRole("button", { name: "Mark Returned" }).click();

    // Loan card should disappear
    await expect(loanCard).not.toBeVisible({ timeout: 10000 });
  });
});
