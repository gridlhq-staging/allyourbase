import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const packageJSON = JSON.parse(readFileSync(join(process.cwd(), "package.json"), "utf8"));

describe("instantsearch demo validation scripts", () => {
  it("rebuilds the repo-local SDK before live browser and Node proof lanes", () => {
    expect(packageJSON.scripts["build:local-sdk"]).toBe("npm --prefix ../../sdk run build");
    expect(packageJSON.scripts["test:node-probe"]).toMatch(/^npm run build:local-sdk && /);
    expect(packageJSON.scripts["test:browser-tests"]).toMatch(/^npm run build:local-sdk && /);
  });
});
