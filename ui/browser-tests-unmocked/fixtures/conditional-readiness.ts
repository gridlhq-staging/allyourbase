import type { TestInfo } from "@playwright/test";

type ReadinessTestInfo = Pick<TestInfo, "attach">;

function forcedReadinessScreens(): Set<string> {
  return new Set(
    (process.env.AYB_BROWSER_FORCE_READINESS_NOT_MET ?? "")
      .split(",")
      .map((screenID) => screenID.trim())
      .filter(Boolean),
  );
}

export async function readinessNotMet(
  testInfo: ReadinessTestInfo,
  screenID: string,
  reason: string,
): Promise<never> {
  await testInfo.attach("readiness-not-met", {
    body: JSON.stringify({
      kind: "READINESS_NOT_MET",
      screenID,
      reason,
    }),
    contentType: "application/json",
  });

  throw new Error(`READINESS_NOT_MET: ${screenID}: ${reason}`);
}

export async function failIfReadinessForced(
  testInfo: ReadinessTestInfo,
  screenID: string,
): Promise<void> {
  if (!forcedReadinessScreens().has(screenID)) {
    return;
  }

  await readinessNotMet(
    testInfo,
    screenID,
    `${screenID} backend forced unavailable for readiness proof`,
  );
}
