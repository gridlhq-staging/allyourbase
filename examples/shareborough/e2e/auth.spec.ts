import { test, expect } from "@playwright/test";
import { uniqueEmail, TEST_PASSWORD, registerUser, loginUser } from "./helpers";

test.describe("Authentication", () => {
  test("shows landing page with sign in and get started links", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByText("Shareborough")).toBeVisible();
    await expect(page.getByRole("link", { name: "Sign in" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Get Started" })).toBeVisible();
  });

  test("can register a new owner account", async ({ page }) => {
    await registerUser(page);
    await expect(page.getByRole("heading", { name: "My Libraries" })).toBeVisible();
    // Verify account menu is visible (Sign out is inside the dropdown)
    await expect(page.getByRole("button", { name: /Account menu/i })).toBeVisible();
  });

  test("can login with existing credentials", async ({ page }) => {
    const email = await registerUser(page);

    // Logout via avatar dropdown
    await page.getByRole("button", { name: /Account menu/i }).click();
    await page.getByText("Sign out").click();
    await page.reload();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible({ timeout: 5000 });

    // Login
    await loginUser(page, email);
    await expect(page.getByRole("heading", { name: "My Libraries" })).toBeVisible();
  });

  test("shows error for invalid credentials", async ({ page }) => {
    await page.goto("/login");
    await page.getByPlaceholder("Email").fill("wrong@example.com");
    await page.getByPlaceholder("Password").fill("wrongpassword");
    await page.getByRole("button", { name: "Sign In" }).click();

    await expect(page.getByText(/something went wrong|invalid|error|incorrect|failed|already/i).first()).toBeVisible({ timeout: 5000 });
  });

  test("can logout", async ({ page }) => {
    await registerUser(page);
    await page.getByRole("button", { name: /Account menu/i }).click();
    await page.getByText("Sign out").click();
    // Reload to verify tokens were cleared — should redirect to login
    await page.reload();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible({ timeout: 5000 });
  });

  test("persists auth across page reload", async ({ page }) => {
    await registerUser(page);
    await page.reload();
    await expect(page.getByRole("heading", { name: "My Libraries" })).toBeVisible({ timeout: 5000 });
  });

  test("shows error when registering with duplicate email", async ({ page }) => {
    const email = await registerUser(page);

    // Logout via avatar dropdown and reload to get to login page
    await page.getByRole("button", { name: /Account menu/i }).click();
    await page.getByText("Sign out").click();
    await page.reload();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible({ timeout: 5000 });

    // Try to register with same email
    await page.goto("/signup");
    await page.getByPlaceholder("Email").fill(email);
    await page.getByPlaceholder("Password").fill(TEST_PASSWORD);
    await page.getByRole("button", { name: "Create Account" }).click();

    await expect(page.getByText(/something went wrong|invalid|error|incorrect|failed|already/i).first()).toBeVisible({ timeout: 5000 });
  });

  test("login page links to signup and vice versa", async ({ page }) => {
    await page.goto("/login");
    await expect(page.getByText("Welcome back")).toBeVisible();
    await expect(page.getByRole("link", { name: "Sign up" })).toBeVisible();

    await page.goto("/signup");
    await expect(page.getByText("Create your account")).toBeVisible();
    await expect(page.getByRole("link", { name: "Sign in" })).toBeVisible();
  });

  test("can login, logout, then login again", async ({ page }) => {
    const email = await registerUser(page);

    // Logout via avatar dropdown
    await page.getByRole("button", { name: /Account menu/i }).click();
    await page.getByText("Sign out").click();
    await page.reload();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible({ timeout: 5000 });

    // Login again
    await loginUser(page, email);
    await expect(page.getByRole("heading", { name: "My Libraries" })).toBeVisible();

    // Logout again via avatar dropdown
    await page.getByRole("button", { name: /Account menu/i }).click();
    await page.getByText("Sign out").click();
    await page.reload();
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible({ timeout: 5000 });

    // Login one more time
    await loginUser(page, email);
    await expect(page.getByRole("heading", { name: "My Libraries" })).toBeVisible();
  });
});
