// ESLint flat config for browser-tests-unmocked
// Enforces human-like interaction patterns in spec files.
// See _dev/BROWSER_TESTING_STANDARDS_3.md for rationale.
import playwright from "eslint-plugin-playwright";
import tseslint from "typescript-eslint";

// Reject direct writes to AYB system tables (_ayb_*) in browser tests. Shared
// across the spec-file and fixture-file config blocks so both surfaces enforce
// the same guard from a single source of truth.
const systemTableWriteSelectors = [
  {
    selector:
      "Literal[value=/\\b(?:INSERT\\s+INTO\\s+_ayb|UPDATE\\s+_ayb|DELETE\\s+FROM\\s+_ayb)/i]",
    message:
      "Direct writes to AYB system tables are not allowed in browser tests. Use a domain fixture or add a local Stage 1 product-gap eslint-disable-next-line.",
  },
  {
    selector:
      "TemplateElement[value.raw=/\\b(?:INSERT\\s+INTO\\s+_ayb|UPDATE\\s+_ayb|DELETE\\s+FROM\\s+_ayb)/i]",
    message:
      "Direct writes to AYB system tables are not allowed in browser tests. Use a domain fixture or add a local Stage 1 product-gap eslint-disable-next-line.",
  },
];

export default [
  {
    // Spec files: strict human-like interaction only
    ...playwright.configs["flat/recommended"],
    files: ["**/*.spec.ts"],
    languageOptions: {
      parser: tseslint.parser,
    },
    rules: {
      ...playwright.configs["flat/recommended"].rules,
      // Intentional environment-conditional skips are a core pattern in this
      // suite — tests skip based on server config (auth mode, feature flags).
      "playwright/no-skipped-test": "off",
      // Environment-adaptive branching (e.g. checking UI state that differs
      // between local/staging/prod) is legitimate for cross-environment E2E.
      "playwright/no-conditional-in-test": "off",
      "playwright/no-conditional-expect": "off",
      // Allow helper functions that wrap expect() (e.g. assertAccessible,
      // assertReadonlyLane, navigateAndScan) to count as assertion-bearing tests.
      // Uses assertFunctionNames for exact names and glob patterns.
      "playwright/expect-expect": ["warn", {
        assertFunctionNames: [
          "assertAccessible",
          "assertOrganizationsPageOutcome",
          "assertTenantsPageOutcome",
          "assertUsagePageOutcome",
          "assertTriggerToggleLifecycle",
          "assertReadonlyLane",
          "assertAllowedTablesLane",
          "assertOwnerMatchLane",
          "navigateAndScan",
        ],
      }],
      "playwright/no-eval": "error",
      "playwright/no-raw-locators": ["error", {
        allowed: ["aside", "tr", 'input[type="file"]', "main", "option"],
      }],
      "playwright/prefer-native-locators": "error",
      "playwright/no-element-handle": "error",
      "playwright/no-page-pause": "error",
      "playwright/no-force-option": "error",
      "no-restricted-syntax": [
        "error",
        {
          selector: "MemberExpression[object.name='request']",
          message: "API calls not allowed in spec files. Move to fixtures.ts.",
        },
        {
          selector: "MemberExpression[property.name='evaluate']",
          message: "page.evaluate() not allowed in spec files.",
        },
        {
          selector: "CallExpression[callee.property.name='waitForTimeout']",
          message: "Arbitrary waits not allowed. Use Playwright auto-waiting.",
        },
        {
          selector: "CallExpression[callee.property.name='dispatchEvent']",
          message:
            "Synthetic events not allowed. Use real user interactions.",
        },
        {
          selector:
            "CallExpression[callee.property.name='setExtraHTTPHeaders']",
          message: "setExtraHTTPHeaders not allowed in spec files.",
        },
        // Spec files must also reject direct AYB system-table writes. Merged in
        // here (rather than a separate **/*.{ts,tsx} block) because ESLint flat
        // config applies rules last-write-wins: a later block redefining
        // no-restricted-syntax would silently drop the spec-only selectors above.
        ...systemTableWriteSelectors,
      ],
    },
  },
  {
    // Fixture and helper files (non-spec): only the system-table-write guard.
    // Excludes **/*.spec.ts so the spec block above remains the sole owner of
    // no-restricted-syntax for spec files.
    files: ["**/*.{ts,tsx}"],
    ignores: ["**/*.spec.ts"],
    languageOptions: {
      parser: tseslint.parser,
    },
    rules: {
      "no-restricted-syntax": ["error", ...systemTableWriteSelectors],
    },
  },
];
