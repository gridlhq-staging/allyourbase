import AxeBuilder from "@axe-core/playwright";
import { test, expect, type Page } from "@playwright/test";
import {
  DEMO_ACCOUNTS,
  arrangeCollectionWriteFailure,
  arrangeDelayedAnonymousBootstrap,
  arrangeEmptyPollBootstrap,
  arrangePollBootstrapFailure,
  createPoll,
  loginWithDemoAccount,
  openCreatePoll,
  openExplicitAuth,
  pollCard,
  registerUser,
  runId,
} from "./helpers";

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

test.describe("Live Polls accessibility states", () => {
  test("a11y: auth sign-in", async ({ page }) => {
    await openExplicitAuth(page);
    await expect(page.getByRole("button", { name: "Sign In", exact: true })).toBeVisible();
    await assertAccessible(page, "auth sign-in");
  });

  test("a11y: auth register", async ({ page }) => {
    await openExplicitAuth(page);
    await page.getByRole("button", { name: "Register" }).click();
    await expect(page.getByRole("button", { name: "Create Account" })).toBeVisible();
    await assertAccessible(page, "auth register");
  });

  test("a11y: auth error", async ({ page }) => {
    await openExplicitAuth(page);
    await page.getByPlaceholder("Email").fill("missing@example.com");
    await page.getByPlaceholder("Password").fill("wrong-password");
    await page.getByRole("button", { name: "Sign In", exact: true }).click();
    await expect(page.getByRole("alert")).toBeVisible();
    await assertAccessible(page, "auth error");
  });

  test("a11y: first-load loading", async ({ page }) => {
    const heldBootstrap = await arrangeDelayedAnonymousBootstrap(page);
    await page.goto("/");
    await heldBootstrap.blocked;
    await expect(page.getByText("Loading...", { exact: true })).toBeVisible();
    await assertAccessible(page, "first-load loading");
    heldBootstrap.release();
  });

  test("a11y: feed anonymous populated", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
    await expect(page.getByTestId("poll-card").first()).toBeVisible({ timeout: 10000 });
    await assertAccessible(page, "feed anonymous populated");
  });

  test("a11y: feed empty", async ({ page }) => {
    await arrangeEmptyPollBootstrap(page);
    await page.goto("/");
    await expect(page.getByText("No polls yet", { exact: true })).toBeVisible();
    await assertAccessible(page, "feed empty");
  });

  test("a11y: feed bootstrap error", async ({ page }) => {
    await registerUser(page);
    const bootstrapFailure = await arrangePollBootstrapFailure(page);
    await page.reload();
    await bootstrapFailure.intercepted;
    await expect(
      page.getByText("Could not load polls. Please refresh.", { exact: true }),
    ).toBeVisible();
    await assertAccessible(page, "feed bootstrap error");
  });

  test("a11y: create form default", async ({ page }) => {
    await registerUser(page);
    await openCreatePoll(page);
    await expect(page.getByRole("heading", { name: "New Poll" })).toBeVisible();
    await assertAccessible(page, "create form default");
  });

  test("a11y: create form extra option", async ({ page }) => {
    await registerUser(page);
    await openCreatePoll(page);
    await page.getByRole("button", { name: "+ Add option" }).click();
    await expect(page.getByPlaceholder("Option 3")).toBeVisible();
    await assertAccessible(page, "create form extra option");
  });

  test("a11y: create validation error", async ({ page }) => {
    await registerUser(page);
    await openCreatePoll(page);
    await page.getByPlaceholder("Ask a question...").fill(`Validation poll ${runId}?`);
    await page.getByPlaceholder("Option 1").fill("Only option");
    await page.getByRole("button", { name: "Create Poll" }).click();
    await expect(page.getByText("At least 2 options required")).toBeVisible();
    await assertAccessible(page, "create validation error");
  });

  test("a11y: create pending", async ({ page }) => {
    await registerUser(page);
    const releaseFailure = await arrangeCollectionWriteFailure(page, "polls");
    await openCreatePoll(page);
    await page.getByPlaceholder("Ask a question...").fill(`Pending poll ${runId}?`);
    await page.getByPlaceholder("Option 1").fill("Yes");
    await page.getByPlaceholder("Option 2").fill("No");
    await page.getByRole("button", { name: "Create Poll" }).click();
    await expect(page.getByRole("button", { name: "Creating..." })).toBeDisabled();
    await assertAccessible(page, "create pending");
    releaseFailure();
  });

  test("a11y: create error", async ({ page }) => {
    await registerUser(page);
    const releaseFailure = await arrangeCollectionWriteFailure(page, "polls");
    await openCreatePoll(page);
    await page.getByPlaceholder("Ask a question...").fill(`Error poll ${runId}?`);
    await page.getByPlaceholder("Option 1").fill("Yes");
    await page.getByPlaceholder("Option 2").fill("No");
    await page.getByRole("button", { name: "Create Poll" }).click();
    releaseFailure();
    await expect(
      page.getByText("Could not create poll. Please try again.", { exact: true }),
    ).toBeVisible();
    await assertAccessible(page, "create error");
  });

  test("a11y: poll open zero votes", async ({ page }) => {
    await registerUser(page);
    const card = await createPoll(page, `Zero votes ${runId}?`, ["Yes", "No"]);
    await expect(card.getByText("0 total votes")).toBeVisible();
    await assertAccessible(page, "poll open zero votes");
  });

  test("a11y: poll voted", async ({ page }) => {
    await registerUser(page);
    const card = await createPoll(page, `Voted poll ${runId}?`, ["Yes", "No"]);
    await card.getByRole("button", { name: /Yes/ }).click();
    await expect(card.getByText("1 total vote")).toBeVisible();
    await assertAccessible(page, "poll voted");
  });

  test("a11y: vote pending", async ({ page }) => {
    await registerUser(page);
    const card = await createPoll(page, `Pending vote ${runId}?`, ["First", "Second"]);
    const releaseFailure = await arrangeCollectionWriteFailure(page, "votes");
    const firstOption = card.getByRole("button", { name: /First/ });
    await firstOption.click();
    await expect(firstOption).toBeDisabled();
    await assertAccessible(page, "vote pending");
    releaseFailure();
  });

  test("a11y: vote error", async ({ page }) => {
    await registerUser(page);
    const card = await createPoll(page, `Error vote ${runId}?`, ["First", "Second"]);
    const releaseFailure = await arrangeCollectionWriteFailure(page, "votes");
    const firstOption = card.getByRole("button", { name: /First/ });
    await firstOption.click();
    releaseFailure();
    await expect(
      card.getByText("Could not record vote. Please try again.", { exact: true }),
    ).toBeVisible();
    await assertAccessible(page, "vote error");
  });

  test("a11y: poll closed owner", async ({ page }) => {
    await registerUser(page);
    const card = await createPoll(page, `Owner closed ${runId}?`, ["Yes", "No"]);
    await card.getByText("Close poll").click();
    await expect(card.getByText("Closed", { exact: true })).toBeVisible();
    await assertAccessible(page, "poll closed owner");
  });

  test("a11y: poll closed non-owner", async ({ browser }) => {
    const ownerContext = await browser.newContext();
    const voterContext = await browser.newContext();
    try {
      const ownerPage = await ownerContext.newPage();
      await loginWithDemoAccount(ownerPage, DEMO_ACCOUNTS[0].email);
      const question = `Non-owner closed ${runId}?`;
      const ownerCard = await createPoll(ownerPage, question, ["Yes", "No"]);
      await ownerCard.getByText("Close poll").click();
      await expect(ownerCard.getByText("Closed", { exact: true })).toBeVisible();

      const voterPage = await voterContext.newPage();
      await loginWithDemoAccount(voterPage, DEMO_ACCOUNTS[1].email);
      const voterCard = pollCard(voterPage, question);
      await expect(voterCard.getByText("Closed", { exact: true })).toBeVisible();
      await assertAccessible(voterPage, "poll closed non-owner");
    } finally {
      await ownerContext.close();
      await voterContext.close();
    }
  });
});
