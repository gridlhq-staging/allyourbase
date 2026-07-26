import { test as base } from "@playwright/test";

export const test = base.extend<{ privateLaunchPage: void }>({
  privateLaunchPage: [async ({ page }, use) => {
    let statusChecks = 0;
    await page.route("**/turnstile/v0/api.js*", async (route) => {
      await route.fulfill({
        contentType: "application/javascript",
        body: `const completeChallenge = () => {
          const form = document.querySelector("form");
          const input = document.createElement("input");
          input.type = "hidden";
          input.name = "cf-turnstile-response";
          input.value = "browser-human-token";
          form.appendChild(input);
          const ready = window.setInterval(() => {
            if (typeof window.turnstileReady === "function") {
              window.clearInterval(ready);
              window.turnstileReady();
            }
          }, 25);
        };
        if (document.readyState === "loading") {
          window.addEventListener("DOMContentLoaded", completeChallenge);
        } else {
          completeChallenge();
        }`,
      });
    });
    await page.route("**/api/try", async (route) => {
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify({ launchToken: "browser-launch", expiresAt: "2027-01-15T08:30:00.000Z" }),
      });
    });
    await page.route("**/api/try/status?launch=browser-launch", async (route) => {
      statusChecks += 1;
      await route.fulfill({
        contentType: "application/json",
        body: JSON.stringify(statusChecks === 1 ? { status: "starting" } : {
          status: "ready",
          adminUrl: "https://private.example/admin?token=signed",
          adminPassword: "temporary-admin-password",
          expiresAt: "2027-01-15T08:30:00.000Z",
        }),
      });
    });
    await use();
  }, { auto: true }],
});

export { expect } from "@playwright/test";
