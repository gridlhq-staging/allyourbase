import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import {
  DEMO_EMAIL,
  delayNextBYOKClear,
  delayNextMovieSearch,
  delayNextNoteEmbed,
  failChatStream,
  failNextAuthLogin,
  failNextMovieSearch,
  failNextNoteEmbed,
  loginWithDemoAccount,
  optOutAnonymousBootstrap,
  recordNextMagicLinkRequest,
  returnEmptyMovieCorpus,
  searchForMovie,
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

async function openSignedInSearch(page: Page): Promise<void> {
  await loginWithDemoAccount(page);
  await expect(page.getByTestId("results-summary")).toContainText("of 250 movies", { timeout: 15000 });
}

async function openSelectedInception(page: Page): Promise<void> {
  await openSignedInSearch(page);
  await searchForMovie(page, "inception", "inception");
  await page.getByTestId("search-result-row-inception").click();
  await expect(page.getByTestId("selected-result-notes-panel")).toBeVisible();
}

test.describe("Movies accessibility states", () => {
  test("a11y: signed-out auth", async ({ page }) => {
    await optOutAnonymousBootstrap(page);
    await page.goto("/");
    await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();
    await assertAccessible(page, "signed-out auth");
  });

  test("a11y: signed-out register", async ({ page }) => {
    await optOutAnonymousBootstrap(page);
    await page.goto("/");
    await page.getByRole("button", { name: "Sign up" }).click();
    await expect(page.getByRole("button", { name: "Create Account" })).toBeVisible();
    await assertAccessible(page, "signed-out register");
  });

  test("a11y: signed-out magic-link success", async ({ page }) => {
    await optOutAnonymousBootstrap(page);
    await page.goto("/");
    await page.getByLabel("Email").fill("movie-a11y-magic@example.test");
    const magicLinkRequest = await recordNextMagicLinkRequest(page);
    await page.getByRole("button", { name: "Email me a magic link" }).click();
    await expect(magicLinkRequest.evidence()).resolves.toMatchObject({
      method: "POST",
      path: "/api/auth/magic-link",
      responseStatus: 200,
    });
    await expect(page.getByRole("status")).toContainText("We sent a magic link");
    await assertAccessible(page, "signed-out magic-link success");
  });

  test("a11y: signed-out auth error", async ({ page }) => {
    await failNextAuthLogin(page);
    await optOutAnonymousBootstrap(page);
    await page.goto("/");
    await page.getByLabel("Email").fill("movie-a11y-auth-error@example.test");
    await page.getByLabel("Password").fill("password123");
    await page.getByRole("button", { name: "Sign In" }).click();
    await expect(page.getByRole("alert")).toContainText("Forced auth failure", { timeout: 15000 });
    await assertAccessible(page, "signed-out auth error");
  });

  test("a11y: initial search loading", async ({ page }) => {
    await delayNextMovieSearch(page);
    await loginWithDemoAccount(page);
    await expect(page.getByTestId("results-summary")).toHaveText("Loading movies...");
    await assertAccessible(page, "initial search loading");
  });

  test("a11y: typed search loading", async ({ page }) => {
    await openSignedInSearch(page);
    await delayNextMovieSearch(page);
    await page.getByPlaceholder("Search movies...").fill("inception");
    await expect(page.getByTestId("results-summary")).toHaveText("Searching movies...");
    await assertAccessible(page, "typed search loading");
  });

  test("a11y: search error and retry", async ({ page }) => {
    await failNextMovieSearch(page);
    await loginWithDemoAccount(page);
    await expect(page.getByRole("alert")).toHaveText("Movie search failed", { timeout: 15000 });
    await expect(page.getByRole("button", { name: "Retry search" })).toBeVisible();
    await assertAccessible(page, "search error and retry");
  });

  test("a11y: corpus empty", async ({ page }) => {
    await returnEmptyMovieCorpus(page);
    await loginWithDemoAccount(page);
    await expect(page.getByTestId("corpus-empty")).toHaveText("No seeded movies found", { timeout: 15000 });
    await assertAccessible(page, "corpus empty");
  });

  test("a11y: no-match search", async ({ page }) => {
    await openSignedInSearch(page);
    await page.getByPlaceholder("Search movies...").fill("xyzzyqwertynomatchxyzzy");
    await expect(page.getByTestId("no-matches")).toHaveText("No movies match your filters", { timeout: 15000 });
    await assertAccessible(page, "no-match search");
  });

  test("a11y: populated results", async ({ page }) => {
    await openSignedInSearch(page);
    await expect(page.getByTestId("search-result-row-inception")).toBeVisible();
    await assertAccessible(page, "populated results");
  });

  test("a11y: genre and decade controls", async ({ page }) => {
    await openSignedInSearch(page);
    await page.getByTestId("genre-facet-Sci-Fi").click();
    await page.getByTestId("decade-filter").selectOption("2010");
    await expect(page.getByTestId("clear-filters")).toBeEnabled();
    await assertAccessible(page, "genre and decade controls");
  });

  test("a11y: selected detail and notes", async ({ page }) => {
    await openSelectedInception(page);
    const noteInput = page.getByPlaceholder("Add a note about this movie...");
    await noteInput.fill("A11y selected note");
    await expect(noteInput).toHaveValue("A11y selected note");
    await assertAccessible(page, "selected detail and notes");
  });

  test("a11y: note saving", async ({ page }) => {
    await openSelectedInception(page);
    await delayNextNoteEmbed(page);
    await page.getByPlaceholder("Add a note about this movie...").fill("A11y saving note");
    await page.getByRole("button", { name: "Save Note" }).click();
    await expect(page.getByRole("button", { name: "Saving..." })).toBeDisabled();
    await assertAccessible(page, "note saving");
  });

  test("a11y: note error", async ({ page }) => {
    await openSelectedInception(page);
    await failNextNoteEmbed(page);
    await page.getByPlaceholder("Add a note about this movie...").fill("A11y failed note");
    await page.getByRole("button", { name: "Save Note" }).click();
    await expect(page.getByRole("alert")).toHaveText("Embed failed: 500", { timeout: 15000 });
    await assertAccessible(page, "note error");
  });

  test("a11y: provider controls", async ({ page }) => {
    await openSignedInSearch(page);
    const providerKeys = page.getByTestId("provider-keys-section");
    const providerSelect = providerKeys.getByRole("combobox");
    await providerSelect.selectOption("anthropic");
    await expect(providerSelect).toHaveValue("anthropic");
    await providerKeys.getByPlaceholder("Vault secret name...").fill("movie_a11y_secret");
    await assertAccessible(page, "provider controls");
  });

  test("a11y: provider clear pending", async ({ page }) => {
    await openSignedInSearch(page);
    await delayNextBYOKClear(page);
    const providerKeys = page.getByTestId("provider-keys-section");
    await providerKeys.getByRole("button", { name: "Clear" }).click();
    await expect(providerKeys.getByRole("button", { name: "Clear" })).toBeDisabled();
    await assertAccessible(page, "provider clear pending");
  });

  test("a11y: provider error", async ({ page }) => {
    await openSignedInSearch(page);
    await page.getByPlaceholder("Vault secret name...").fill("nonexistent_local_secret");
    await page.getByRole("button", { name: "Set" }).click();
    await expect(page.getByRole("alert")).toContainText("BYOK set failed", { timeout: 15000 });
    await assertAccessible(page, "provider error");
  });

  test("a11y: chat success", async ({ page }) => {
    await openSignedInSearch(page);
    await page.getByPlaceholder("Ask about movies...").fill("Summarize inception");
    await page.getByRole("button", { name: "Send" }).click();
    await expect(page.getByText("Local stub response: Summarize inception")).toBeVisible({ timeout: 15000 });
    await assertAccessible(page, "chat success");
  });

  test("a11y: chat streamed in progress", async ({ page }) => {
    await openSignedInSearch(page);
    await page.getByPlaceholder("Ask about movies...").fill("Summarize inception");
    await page.getByRole("button", { name: "Send" }).click();
    await expect(page.getByPlaceholder("Ask about movies...")).toBeDisabled();
    const chatSection = page.getByTestId("chat-section");
    await expect(chatSection).toContainText("Local stub response:", { timeout: 15000 });
    await expect(chatSection).not.toContainText("Local stub response: Summarize inception", { timeout: 100 });
    await expect(chatSection).toContainText("Local stub response: Summarize", { timeout: 15000 });
    await expect(chatSection).not.toContainText("Local stub response: Summarize inception", { timeout: 100 });
    await assertAccessible(page, "chat streamed in progress");
  });

  test("a11y: chat error", async ({ page }) => {
    await failChatStream(page);
    await openSignedInSearch(page);
    await page.getByPlaceholder("Ask about movies...").fill("Summarize inception");
    await page.getByRole("button", { name: "Send" }).click();
    await expect(page.getByText("Error: chat request failed.")).toBeVisible({ timeout: 15000 });
    await assertAccessible(page, "chat error");
  });

  test("a11y: demo account control", async ({ page }) => {
    await optOutAnonymousBootstrap(page);
    await page.goto("/");
    await page.getByRole("button", { name: DEMO_EMAIL }).click();
    await expect(page.getByLabel("Email")).toHaveValue(DEMO_EMAIL);
    await assertAccessible(page, "demo account control");
  });
});
