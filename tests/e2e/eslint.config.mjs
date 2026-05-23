import playwright from "eslint-plugin-playwright";
import tseslint from "typescript-eslint";

export default [
  {
    ignores: ["node_modules/**", "test-results/**", "playwright-report/**"],
  },
  ...tseslint.configs.recommended.map((config) => ({
    ...config,
    files: ["**/*.ts"],
  })),
  {
    ...playwright.configs["flat/recommended"],
    files: ["**/*.spec.ts"],
    rules: {
      ...playwright.configs["flat/recommended"].rules,
      "playwright/no-eval": "error",
      "playwright/no-raw-locators": "error",
      "playwright/prefer-native-locators": "warn",
      "playwright/no-element-handle": "error",
      "playwright/no-force-option": "error",
      "playwright/no-page-pause": "error",
      "playwright/no-wait-for-timeout": "error",
      "no-restricted-syntax": [
        "error",
        {
          selector: "CallExpression[callee.property.name='waitForTimeout']",
          message: "Use assertion timeouts instead of waitForTimeout.",
        },
        {
          selector: "CallExpression[callee.property.name='dispatchEvent']",
          message: "Do not use dispatchEvent; use real user interactions.",
        },
        {
          selector: "CallExpression[callee.property.name='setExtraHTTPHeaders']",
          message: "Do not modify transport headers in browser specs.",
        },
        {
          selector: "CallExpression[callee.object.name='request']",
          message: "Do not use request fixture in spec files; place API calls in fixtures.ts.",
        }
      ],
      "@typescript-eslint/no-unused-vars": "off"
    }
  },
  {
    files: ["fixtures.ts"],
    rules: {
      "no-restricted-syntax": "off",
      "playwright/no-raw-locators": "off"
    }
  }
];
