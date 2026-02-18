import { test, expect } from "@playwright/test";

test.describe("Production Smoke Tests", () => {
  test("Landing page loads with Sign In and Get Started buttons", async ({ page }) => {
    await page.goto("/");

    // Wait for React to render
    await page.waitForLoadState("networkidle");

    // Check title
    await expect(page).toHaveTitle(/Shareborough/);

    // Check for Sign In button
    const signInButton = page.getByRole("link", { name: "Sign in" });
    await expect(signInButton).toBeVisible();
    await expect(signInButton).toHaveAttribute("href", "/login");

    // Check for Get Started button
    const getStartedButton = page.getByRole("link", { name: "Get Started" });
    await expect(getStartedButton).toBeVisible();
    await expect(getStartedButton).toHaveAttribute("href", "/signup");

    // Check hero text
    await expect(page.getByRole("heading", { name: /Lend stuff to/ })).toBeVisible();
  });

  test("Sign In button navigates to login page", async ({ page }) => {
    await page.goto("/");
    await page.waitForLoadState("networkidle");

    const signInButton = page.getByRole("link", { name: "Sign in" });
    await signInButton.click();

    await expect(page).toHaveURL("/login");
    await expect(page.getByRole("heading", { name: "Welcome back" })).toBeVisible();
  });

  test("Get Started button navigates to signup page", async ({ page }) => {
    await page.goto("/");
    await page.waitForLoadState("networkidle");

    const getStartedButton = page.getByRole("link", { name: "Get Started" });
    await getStartedButton.click();

    await expect(page).toHaveURL("/signup");
    await expect(page.getByRole("heading", { name: "Create your account" })).toBeVisible();
  });

  test("Start Your Library button navigates to signup", async ({ page }) => {
    await page.goto("/");
    await page.waitForLoadState("networkidle");

    const startLibraryButton = page.getByRole("link", { name: "Start Your Library" });
    await expect(startLibraryButton).toBeVisible();
    await startLibraryButton.click();

    await expect(page).toHaveURL("/signup");
  });

  test("API health check is accessible", async ({ request }) => {
    const response = await request.get("https://api.shareborough.com/health");
    expect(response.ok()).toBeTruthy();
    const body = await response.json();
    expect(body).toEqual({ status: "ok" });
  });
});
