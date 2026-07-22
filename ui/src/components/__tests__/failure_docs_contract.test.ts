import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const FAILURE_PRESENTATION_OWNERS = [
  "../SqlEditor.tsx",
  "../StorageBrowser.tsx",
  "../Login.tsx",
  "../RlsPolicies.tsx",
  "../ContentRouter.tsx",
  "../ErrorNotice.tsx",
] as const;

describe("failure documentation ownership", () => {
  it.each(FAILURE_PRESENTATION_OWNERS)(
    "%s delegates the public documentation origin to docsUrl",
    (relativePath) => {
      const source = readFileSync(resolve(__dirname, relativePath), "utf8");
      expect(source).not.toContain("https://allyourbase.io");
    },
  );
});
