// ESLint config for movies demo source and Playwright e2e tests.
// The e2e section enforces BROWSER_TESTING_STANDARDS_3.md — no shortcuts in spec files.
import playwright from "eslint-plugin-playwright";
import tseslint from "typescript-eslint";

export default [
  ...tseslint.configs.recommended.map((config) => ({
    ...config,
    files: ["src/**/*.{ts,tsx}", "e2e/**/*.ts"],
  })),
  {
    ...playwright.configs["flat/recommended"],
    files: ["e2e/**/*.spec.ts"],
    rules: {
      ...playwright.configs["flat/recommended"].rules,

      // Ban page.evaluate and friends.
      "playwright/no-eval": "error",

      // Ban raw CSS/XPath locators — use getByRole/getByText/getByLabel.
      "playwright/no-raw-locators": "error",

      // Prefer native locators.
      "playwright/prefer-native-locators": "error",

      // Ban deprecated page.$() API.
      "playwright/no-element-handle": "error",

      // Ban { force: true } on clicks.
      "playwright/no-force-option": "error",

      // Ban page.pause() (debugging leftover).
      "playwright/no-page-pause": "error",

      // Ban API calls, waitForTimeout, dispatchEvent, setExtraHTTPHeaders in specs.
      "no-restricted-syntax": [
        "error",
        {
          selector: "MemberExpression[object.name='request']",
          message: "API calls not allowed in spec files. Move to helpers.ts.",
        },
        {
          selector: "MemberExpression[property.name='evaluate']",
          message: "page.evaluate() not allowed in spec files.",
        },
        {
          selector: "CallExpression[callee.property.name='waitForTimeout']",
          message: "Use assertion timeout instead of waitForTimeout.",
        },
        {
          selector: "CallExpression[callee.property.name='dispatchEvent']",
          message: "Do not use dispatchEvent — simulate real user interactions.",
        },
        {
          selector: "CallExpression[callee.property.name='setExtraHTTPHeaders']",
          message: "Do not set HTTP headers — users can't do this in the UI.",
        },
      ],

      // Suppress TS rules that conflict with Playwright patterns.
      "@typescript-eslint/no-unused-vars": "off",
    },
  },
  {
    // Helper files keep Playwright-specific exemptions but must not reintroduce fixed-delay route sleeps.
    files: ["e2e/helpers.ts"],
    rules: {
      "playwright/no-raw-locators": "off",
      "no-restricted-syntax": [
        "error",
        {
          selector:
            "NewExpression[callee.name='Promise'] CallExpression:not([arguments.1.value=0]):matches([callee.name='setTimeout'], [callee.property.name='setTimeout'])",
          message:
            "Use blockNextMovieSearch or another release-on-assertion gate instead of fixed-delay helper sleeps.",
        },
      ],
    },
  },
];
