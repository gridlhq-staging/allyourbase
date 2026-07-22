import { test, expect } from "@playwright/test";
import {
  arrangeCollectionWriteFailure,
  arrangeDelayedAnonymousBootstrap,
  arrangeEmptyPollBootstrap,
  arrangePollBootstrapFailure,
  createPoll,
  openCreatePoll,
  registerUser,
  runId,
} from "./helpers";

test.describe("Live poll states", () => {
  test("shows a deterministic first-load state", async ({ page }) => {
    const heldBootstrap = await arrangeDelayedAnonymousBootstrap(page);

    await page.goto("/");

    // Prove the delayed anonymous bootstrap request is actually blocked before
    // asserting the loading UI, so this cannot pass on the initial auth splash alone.
    await heldBootstrap.blocked;
    await expect(page.getByText("Loading...", { exact: true })).toBeVisible();

    heldBootstrap.release();
    await expect(page.getByRole("button", { name: "Sign out" })).toBeVisible();
  });

  test("shows the empty poll list without bootstrap seeding", async ({ page }) => {
    await arrangeEmptyPollBootstrap(page);

    await page.goto("/");

    await expect(page.getByText("No polls yet", { exact: true })).toBeVisible();
    await expect(page.getByText("Create the first one!", { exact: true })).toBeVisible();
    await expect(page.getByTestId("poll-card")).toHaveCount(0);
  });

  test("shows a stable bootstrap failure message", async ({ page }) => {
    await registerUser(page);
    await arrangePollBootstrapFailure(page);

    await page.reload();

    await expect(
      page.getByText("Could not load polls. Please refresh.", { exact: true }),
    ).toBeVisible();
  });

  test("shows create progress and a stable failure message", async ({ page }) => {
    await registerUser(page);
    const releaseFailure = await arrangeCollectionWriteFailure(page, "polls");
    await openCreatePoll(page);
    await page.getByPlaceholder("Ask a question...").fill(`Failure poll ${runId}?`);
    await page.getByPlaceholder("Option 1").fill("Yes");
    await page.getByPlaceholder("Option 2").fill("No");

    await page.getByRole("button", { name: "Create Poll" }).click();

    await expect(page.getByRole("button", { name: "Creating..." })).toBeDisabled();
    releaseFailure();
    await expect(
      page.getByText("Could not create poll. Please try again.", { exact: true }),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Create Poll" })).toBeEnabled();
  });

  test("disables voting in progress and shows a stable failure message", async ({ page }) => {
    await registerUser(page);
    const card = await createPoll(page, `Failure vote ${runId}?`, ["First", "Second"]);
    const releaseFailure = await arrangeCollectionWriteFailure(page, "votes");
    const firstOption = card.getByRole("button", { name: /First/ });

    await firstOption.click();

    await expect(firstOption).toBeDisabled();
    releaseFailure();
    await expect(
      card.getByText("Could not record vote. Please try again.", { exact: true }),
    ).toBeVisible();
    await expect(firstOption).toBeEnabled();
  });
});
