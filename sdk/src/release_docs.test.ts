import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const sdkRoot = resolve(__dirname, "..");

function readSDKFile(relativePath: string): string {
  return readFileSync(resolve(sdkRoot, relativePath), "utf8");
}

describe("0.2.0 release metadata", () => {
  it("publishes the package metadata as 0.2.0 without changing package shape", () => {
    const manifest = JSON.parse(readSDKFile("package.json")) as {
      name: string;
      version: string;
      main: string;
      module: string;
      types: string;
      exports: Record<string, unknown>;
      files: string[];
      scripts: Record<string, string>;
    };

    expect(manifest.name).toBe("@allyourbase/js");
    expect(manifest.version).toBe("0.2.0");
    expect(manifest.main).toBe("dist/index.cjs");
    expect(manifest.module).toBe("dist/index.js");
    expect(manifest.types).toBe("dist/index.d.ts");
    expect(manifest.exports["."]).toEqual({
      import: {
        types: "./dist/index.d.ts",
        default: "./dist/index.js",
      },
      require: {
        types: "./dist/index.d.cts",
        default: "./dist/index.cjs",
      },
    });
    expect(manifest.exports["./instantsearch"]).toEqual({
      import: {
        types: "./dist/instantsearch.d.ts",
        default: "./dist/instantsearch.js",
      },
      require: {
        types: "./dist/instantsearch.d.cts",
        default: "./dist/instantsearch.cjs",
      },
    });
    expect(manifest.files).toEqual(["dist"]);
    expect(manifest.scripts.prepublishOnly).toBe("npm run build");
  });

  it("builds the InstantSearch subpath from its single SDK source owner", () => {
    const tsupConfig = readSDKFile("tsup.config.ts");

    expect(tsupConfig).toContain('entry: ["src/index.ts", "src/instantsearch.ts"]');
  });

  it("documents only the shipped search and passkey surfaces for 0.2.0", () => {
    const changelog = readSDKFile("CHANGELOG.md");
    const readme = readSDKFile("README.md");

    expect(changelog).toContain("## 0.2.0");
    expect(changelog).toContain("search");
    expect(changelog).toContain("fuzzy");
    expect(changelog).toContain("typoThreshold");
    expect(changelog).toContain("boolean `highlight`");
    expect(changelog).toContain("highlight: true");
    expect(changelog).toContain("_highlight");
    expect(changelog).toContain("facets");
    expect(changelog).toContain("semantic");
    expect(changelog).toContain("semanticQuery");
    expect(changelog).toContain("nearest");
    expect(changelog).toContain("vectorColumn");
    expect(changelog).toContain("distance");
    expect(changelog).toContain("beginWebAuthnLogin");
    expect(changelog).toContain("finishWebAuthnLogin");
    expect(changelog).toContain("signInWithPasskey");
    expect(changelog).toContain("enrollPasskey");
    expect(changelog).toContain("verifyPasskey");
    expect(changelog).toContain("@allyourbase/js/instantsearch");
    expect(changelog).toContain("objectIDField");
    expect(changelog).toContain("records.list");
    expect(changelog).toContain("empty query");
    expect(changelog).toContain("searchForFacetValues");

    expect(readme).toContain("SearchHit");
    expect(readme).toContain("ayb.records.list<SearchHit");
    expect(readme).toContain("_highlight");
    expect(readme).toContain("signInWithPasskey");
    expect(readme).toContain("beginWebAuthnLogin");
    expect(readme).toContain("finishWebAuthnLogin");
    expect(readme).toContain("enrollPasskey");
    expect(readme).toContain("verifyPasskey");
    expect(readme).toContain("FacetCounts");
    expect(readme).toContain("WebAuthnLoginBeginResponse");
    expect(readme).toContain("@allyourbase/js/instantsearch");
    expect(readme).toContain("createInstantSearchClient");
    expect(readme).toContain("objectIDField");
    expect(readme).toContain("facetFilters");
    expect(readme).toContain("searchForFacetValues");
  });
});
