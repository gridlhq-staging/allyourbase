import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

function readCspEnforcingSpec(): string {
  return readFileSync(
    resolve(
      __dirname,
      "..",
      "..",
      "browser-tests-unmocked",
      "smoke",
      "csp-enforcing.spec.ts",
    ),
    "utf8",
  );
}

describe("CSP enforcing fixture cleanup contract", () => {
  it("tracks each fixture before a later setup step can fail", () => {
    const spec = readCspEnforcingSpec();

    expect(spec).toMatch(
      /const screenFixtures = createDashboardScreenFixtures\(testInfo\);[\s\S]*?try \{[\s\S]*?await prepareDashboardScreenFixtures\([\s\S]*?screenFixtures,[\s\S]*?\);/,
    );
    expect(spec).toMatch(
      /fixtures\.functionName = await prepareFunctionFixture\([\s\S]*?fixtures\.graphql = await prepareGraphqlFixture\([\s\S]*?await prepareStorageFixture\(request,\s*adminToken,\s*fixtures\);/,
    );
    expect(spec).toMatch(
      /const fileName = `csp-preview-\$\{fixtures\.runID\}\.png`;[\s\S]*?fixtures\.storageImageFileName = fileName;[\s\S]*?await seedFile\(/,
    );
  });
});
