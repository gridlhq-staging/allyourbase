import { expect, test } from "@playwright/test";
import {
  DEMO_EMAIL,
  delayNextMovieSearch,
  failNextAuthLogin,
  failChatStream,
  failNextLogout,
  failNextMovieSearch,
  failNextNoteEmbed,
  loginWithDemoAccount,
  optOutAnonymousBootstrap,
  recordNextMagicLinkRequest,
  returnEmptyMovieCorpus,
  searchForMovie,
} from "./helpers";

test("movie corpus load renders a browser-visible loading state before seeded rows", async ({ page }) => {
  // Real local demo searches can resolve before the browser observes the
  // transient state, so this arrange-side route delay makes the contract
  // deterministic without changing the user action path.
  await delayNextMovieSearch(page);

  await loginWithDemoAccount(page);

  await expect(page.getByTestId("results-summary")).toHaveText("Loading movies...");
  await expect(page.getByTestId("search-result-row-inception")).toBeVisible({ timeout: 15000 });
  await expect(page.getByTestId("search-result-title-inception")).toHaveText("Inception");
  await expect(page.getByTestId("search-result-year-inception")).toHaveText("2010");
  await expect(page.getByTestId("search-result-genre-inception")).toHaveText("Sci-Fi");
  await expect(page.getByTestId("search-result-overview-inception")).toHaveText(
    "A thief enters dreams to steal secrets and perform a final heist inside layered realities.",
  );
});

test("typed search renders searching status before refreshed results", async ({ page }) => {
  await loginWithDemoAccount(page);
  await expect(page.getByTestId("results-summary")).toContainText("of 250 movies", { timeout: 15000 });

  // Debounced search-as-you-type is normally faster than a reliable visual
  // assertion; this arrange-side delay pins the in-flight browser state.
  await delayNextMovieSearch(page);
  await page.getByPlaceholder("Search movies...").fill("inception");

  await expect(page.getByTestId("results-summary")).toHaveText("Searching movies...");
  await expect(page.getByTestId("search-result-row-inception")).toBeVisible({ timeout: 15000 });
});

test("search API failure renders retry affordance with exact error copy", async ({ page }) => {
  await failNextMovieSearch(page);

  await loginWithDemoAccount(page);

  await expect(page.getByRole("alert")).toHaveText("Movie search failed", { timeout: 15000 });
  await expect(page.getByRole("button", { name: "Retry search" })).toBeVisible();
  await page.getByRole("button", { name: "Retry search" }).click();
  await expect(page.getByTestId("search-result-row-inception")).toBeVisible({ timeout: 15000 });
});

test("empty seeded corpus renders the empty-corpus state", async ({ page }) => {
  await returnEmptyMovieCorpus(page);

  await loginWithDemoAccount(page);

  await expect(page.getByTestId("results-summary")).toHaveText("Showing 0 of 0 movies", { timeout: 15000 });
  await expect(page.getByTestId("corpus-empty")).toHaveText("No seeded movies found");
  await expect(page.getByTestId("genre-facet-group")).toBeHidden();
});

test("active no-match filters render the clearable no-results state", async ({ page }) => {
  await loginWithDemoAccount(page);
  await expect(page.getByTestId("results-summary")).toContainText("of 250 movies", { timeout: 15000 });

  await page.getByPlaceholder("Search movies...").fill("xyzzyqwertynomatchxyzzy");

  await expect(page.getByTestId("no-matches")).toHaveText("No movies match your filters", { timeout: 15000 });
  await expect(page.getByRole("button", { name: "Clear filters" })).toBeEnabled();
  await page.getByRole("button", { name: "Clear filters" }).click();
  await expect(page.getByTestId("results-summary")).toContainText("of 250 movies", { timeout: 15000 });
});

test("signed-out auth surface exposes login, registration, guest, and magic-link states", async ({ page }) => {
  await optOutAnonymousBootstrap(page);
  await page.goto("/");

  await expect(page.getByRole("heading", { name: "Movies Demo" })).toBeVisible();
  await expect(page.getByText("Powered by Allyourbase")).toBeVisible();
  await expect(page.getByLabel("Email")).toBeVisible();
  await expect(page.getByLabel("Password")).toBeVisible();
  await expect(page.getByRole("button", { name: "Sign In" })).toBeDisabled();
  await expect(page.getByText("Demo accounts")).toBeVisible();
  await expect(page.getByRole("button", { name: "alice@demo.test" })).toBeVisible();
  await expect(page.getByRole("button", { name: "bob@demo.test" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Continue as Guest" })).toBeVisible();

  await page.getByRole("button", { name: "Sign up" }).click();
  await expect(page.getByRole("button", { name: "Create Account" })).toBeVisible();
  await expect(page.getByText("Already have an account?")).toBeVisible();
  await page.getByRole("button", { name: "Sign in" }).click();

  await page.getByLabel("Email").fill("magic-state@example.test");
  const magicLinkRequest = await recordNextMagicLinkRequest(page);
  await page.getByRole("button", { name: "Email me a magic link" }).click();
  await expect(magicLinkRequest.evidence()).resolves.toEqual({
    method: "POST",
    path: "/api/auth/magic-link",
    jsonBody: { email: "magic-state@example.test" },
    responseStatus: 200,
  });
  await expect(page.getByRole("status")).toHaveText(
    "We sent a magic link to magic-state@example.test. Check your inbox.",
    { timeout: 15000 },
  );
});

test("magic-link request capture reports malformed JSON without timing out", async ({ page }) => {
  await page.goto("/");
  const magicLinkRequest = await recordNextMagicLinkRequest(page);

  await page.addScriptTag({
    content: `
      void fetch("/api/auth/magic-link", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: "not-json",
      });
    `,
  });

  await expect(magicLinkRequest.evidence()).rejects.toThrow("capture magic-link request");
});

test("authentication failure stays on the signed-out form with exact alert copy", async ({ page }) => {
  await failNextAuthLogin(page);

  await optOutAnonymousBootstrap(page);
  await page.goto("/");
  await page.getByLabel("Email").fill("login-failure@example.test");
  await page.getByLabel("Password").fill("password123");
  await page.getByRole("button", { name: "Sign In" }).click();

  await expect(page.getByRole("alert")).toContainText("Forced auth failure", { timeout: 15000 });
  await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Continue as Guest" })).toBeVisible();
});

test("demo account login and logout return to the signed-out form", async ({ page }) => {
  await optOutAnonymousBootstrap(page);
  await page.goto("/");

  await page.getByRole("button", { name: DEMO_EMAIL }).click();
  await expect(page.getByLabel("Email")).toHaveValue(DEMO_EMAIL);
  await page.getByRole("button", { name: "Sign In" }).click();

  await expect(page.getByTestId("user-email")).toHaveText(DEMO_EMAIL, { timeout: 15000 });
  await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();

  await page.getByRole("button", { name: "Sign out" }).click();

  await expect(page.getByRole("button", { name: "Sign In" })).toBeVisible({ timeout: 15000 });
  await expect(page.getByRole("button", { name: DEMO_EMAIL })).toBeVisible();
});

test("logout failure preserves the signed-in movie surface with retryable alert", async ({ page }) => {
  await loginWithDemoAccount(page);
  await expect(page.getByTestId("user-email")).toHaveText(DEMO_EMAIL, { timeout: 15000 });
  await failNextLogout(page);

  await page.getByRole("button", { name: "Sign out" }).click();

  await expect(page.getByRole("alert")).toHaveText("Sign out failed. Please try again.", { timeout: 15000 });
  await expect(page.getByTestId("user-email")).toHaveText(DEMO_EMAIL);
  await expect(page.getByRole("button", { name: "Sign out" })).toBeEnabled();
});

test("guest sign-in renders the anonymous upgrade copy", async ({ page }) => {
  await page.goto("/");

  await expect(page.getByText("You're browsing as a guest. Add an email and password to keep your data.")).toBeVisible({
    timeout: 15000,
  });
  await expect(page.getByRole("button", { name: "Continue as Guest" })).toBeHidden();
  await expect(page.getByRole("button", { name: "Upgrade Account" })).toBeVisible();
});

test("note-save failure keeps the selected movie note draft visible", async ({ page }) => {
  await loginWithDemoAccount(page);
  await searchForMovie(page, "inception", "inception");
  await failNextNoteEmbed(page);

  await page.getByTestId("search-result-row-inception").click();
  const noteInput = page.getByPlaceholder("Add a note about this movie...");
  await noteInput.fill("Note that should remain after failure");
  await page.getByRole("button", { name: "Save Note" }).click();

  await expect(page.getByRole("alert")).toHaveText("Embed failed: 500", { timeout: 15000 });
  await expect(noteInput).toHaveValue("Note that should remain after failure");
  await expect(page.getByRole("button", { name: "Save Note" })).toBeEnabled();
});

test("chat streams incremental assistant text before final response", async ({ page }) => {
  await loginWithDemoAccount(page);
  await expect(page.getByText("Ask a question about movies...")).toBeVisible();

  await page.getByPlaceholder("Ask about movies...").fill("Summarize inception");
  await page.getByRole("button", { name: "Send" }).click();

  await expect(page.getByText("Summarize inception")).toBeVisible();
  await expect(page.getByPlaceholder("Ask about movies...")).toBeDisabled();
  await expect(page.getByRole("button", { name: "Send" })).toBeDisabled();
  const chatSection = page.getByTestId("chat-section");
  await expect(chatSection).toContainText("Local stub response:", { timeout: 15000 });
  await expect(chatSection).not.toContainText("Local stub response: Summarize inception", { timeout: 100 });
  await expect(chatSection).toContainText("Local stub response: Summarize", { timeout: 15000 });
  await expect(chatSection).not.toContainText("Local stub response: Summarize inception", { timeout: 100 });
  await expect(chatSection).toContainText("Local stub response: Summarize inception", { timeout: 15000 });
  await expect(page.getByPlaceholder("Ask about movies...")).toBeEnabled();
});

test("chat backend failure renders an assistant error in the transcript", async ({ page }) => {
  // The shared Playwright webServer always starts the fake Ollama dependency
  // on a fixed port, so per-spec dead-port induction is unavailable without
  // restarting the runtime. Route fulfillment keeps the failure in Arrange.
  await failChatStream(page);

  await loginWithDemoAccount(page);
  await page.getByPlaceholder("Ask about movies...").fill("Summarize inception");
  await page.getByRole("button", { name: "Send" }).click();

  await expect(page.getByText("Summarize inception")).toBeVisible();
  await expect(page.getByText("Error: chat request failed.")).toBeVisible({ timeout: 15000 });
  await expect(page.getByPlaceholder("Ask about movies...")).toBeEnabled();
});
