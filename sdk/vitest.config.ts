import { defineConfig } from "vitest/config";

const shouldExcludeIntegration =
  process.env.npm_lifecycle_event !== "test:integration" &&
  !process.argv.some((arg) => /integration.*\.test\.ts/.test(arg));

export default defineConfig({
  test: {
    exclude: shouldExcludeIntegration
      ? ["**/integration*.test.ts", "**/node_modules/**"]
      : ["**/node_modules/**"],
  },
});
