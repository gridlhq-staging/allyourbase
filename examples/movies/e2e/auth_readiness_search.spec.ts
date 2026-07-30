import { expect, test } from "@playwright/test";
import {
  loginWithDemoAccountAndExposedClient,
  recordNextMovieSearchRequest,
  triggerSameAccountAuthedReadyDip,
} from "./helpers";

test("typed search survives retained-token auth readiness dip", async ({ page }) => {
  await loginWithDemoAccountAndExposedClient(page);
  await expect(page.getByTestId("results-summary")).toContainText(/of 250 movies/, { timeout: 15000 });

  const searchInput = page.getByPlaceholder("Search movies...");
  await searchInput.fill("inception");
  await expect(searchInput).toHaveValue("inception");
  await expect(page.getByTestId("search-result-row-inception")).toBeVisible({ timeout: 15000 });

  const recoveredSearch = await recordNextMovieSearchRequest(page);
  await triggerSameAccountAuthedReadyDip(page);

  await expect(searchInput).toHaveValue("inception");
  await expect(recoveredSearch.evidence()).resolves.toMatchObject({ search: "inception" });
  await expect(page.getByTestId("search-result-row-inception")).toBeVisible({ timeout: 15000 });
});
