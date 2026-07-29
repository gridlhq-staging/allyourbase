#!/usr/bin/env bash
set -euo pipefail

# The matrix doc lives alongside this script (like scripts/allowlist-oversized.txt)
# so the CI gate, which runs on the synced staging/prod mirrors, can find it.
# _dev/ is dev-repo-only and is not synced to those mirrors.
readonly DEFAULT_MATRIX_PATH="scripts/COVERAGE_MATRIX.md"
readonly MATRIX_PATH="${COVERAGE_MATRIX_PATH:-$DEFAULT_MATRIX_PATH}"
readonly DEFAULT_LAYOUT_TYPES_PATH="ui/src/components/layout-types.ts"
readonly LAYOUT_TYPES_PATH="${COVERAGE_MATRIX_LAYOUT_TYPES_PATH:-$DEFAULT_LAYOUT_TYPES_PATH}"
readonly DEFAULT_ADMIN_VIEWS_PATH="ui/src/screens/registry.ts"
readonly ADMIN_VIEWS_PATH="${COVERAGE_MATRIX_ADMIN_VIEWS_PATH:-$DEFAULT_ADMIN_VIEWS_PATH}"

if [[ ! -f "$MATRIX_PATH" ]]; then
  echo "Coverage matrix not found: $MATRIX_PATH" >&2
  exit 1
fi

if [[ ! -f "$LAYOUT_TYPES_PATH" ]]; then
  echo "Layout types source not found: $LAYOUT_TYPES_PATH" >&2
  exit 1
fi

if [[ ! -f "$ADMIN_VIEWS_PATH" ]]; then
  echo "Admin views source not found: $ADMIN_VIEWS_PATH" >&2
  exit 1
fi

extract_metric_count() {
  local metric_name="$1"

  awk -F'|' -v metric="$metric_name" '
    function trim(value) {
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      return value
    }

    /^##[[:space:]]+Gap Summary/ {
      in_gap_summary = 1
      next
    }

    in_gap_summary && /^##[[:space:]]+/ {
      in_gap_summary = 0
    }

    in_gap_summary && /^\|/ {
      left = trim($2)
      right = trim($3)

      if (left == metric) {
        if (match(right, /[0-9]+/)) {
          print substr(right, RSTART, RLENGTH)
          exit
        }
      }
    }
  ' "$MATRIX_PATH"
}

assert_integer() {
  local label="$1"
  local value="$2"

  if [[ -z "$value" || ! "$value" =~ ^[0-9]+$ ]]; then
    echo "Unable to parse metric '$label' from $MATRIX_PATH" >&2
    exit 1
  fi
}

smoke_none_count="$(extract_metric_count "Smoke = none")"
smoke_heading_only_count="$(extract_metric_count "Smoke = heading-only")"
crud_missing_full_count="$(extract_metric_count "CRUD-capable views missing full lifecycle")"
mocked_coverage_missing_count="$(extract_metric_count "Views missing mocked coverage")"

assert_integer "Smoke = none" "$smoke_none_count"
assert_integer "Smoke = heading-only" "$smoke_heading_only_count"
assert_integer "CRUD-capable views missing full lifecycle" "$crud_missing_full_count"
assert_integer "Views missing mocked coverage" "$mocked_coverage_missing_count"

node - "$MATRIX_PATH" "$LAYOUT_TYPES_PATH" "$ADMIN_VIEWS_PATH" <<'NODE'
const fs = require("fs");

const matrixPath = process.argv[2];
const layoutTypesPath = process.argv[3];
const adminViewsPath = process.argv[4];

const layoutSource = fs.readFileSync(layoutTypesPath, "utf8");
const adminViewsSource = fs.readFileSync(adminViewsPath, "utf8");
const matrixSource = fs.readFileSync(matrixPath, "utf8");

function parseStringLiteralArray(source, constName) {
  const pattern = new RegExp(
    `const\\s+${constName}\\s*=\\s*\\[([\\s\\S]*?)\\]\\s+as\\s+const`,
  );
  const match = source.match(pattern);
  if (!match) {
    return [];
  }

  return [...match[1].matchAll(/"([^"]+)"/g)].map((entry) => entry[1]);
}

function parseStringLiteralUnion(source, typeName) {
  const pattern = new RegExp(`type\\s+${typeName}\\s*=\\s*([\\s\\S]*?);`);
  const match = source.match(pattern);
  if (!match) {
    return [];
  }

  return [...match[1].matchAll(/"([^"]+)"/g)].map((entry) => entry[1]);
}

const uniqueViews = [
  ...new Set([
    ...(
      parseStringLiteralArray(layoutSource, "DATA_VIEWS").length > 0
        ? parseStringLiteralArray(layoutSource, "DATA_VIEWS")
        : parseStringLiteralUnion(layoutSource, "DataView")
    ),
    ...parseStringLiteralArray(adminViewsSource, "ADMIN_VIEWS"),
  ]),
];

if (uniqueViews.length === 0) {
  console.error(
    `Unable to parse view inventory from ${layoutTypesPath} and ${adminViewsPath}`,
  );
  process.exit(1);
}

const matrixSection = matrixSource.split("## Gap Summary")[0];
const matrixViews = [...matrixSection.matchAll(/\|\s*`([^`]+)`\s*\|/g)].map(
  (match) => match[1],
);
const uniqueMatrixViews = [...new Set(matrixViews)];

const missingViews = uniqueViews.filter(
  (view) => !uniqueMatrixViews.includes(view),
);
const extraViews = uniqueMatrixViews.filter((view) => !uniqueViews.includes(view));

if (missingViews.length > 0 || extraViews.length > 0) {
  if (missingViews.length > 0) {
    console.error(
      `Coverage matrix missing views from source inventories: ${missingViews.join(", ")}`,
    );
  }
  if (extraViews.length > 0) {
    console.error(
      `Coverage matrix has unknown views not present in source inventories: ${extraViews.join(", ")}`,
    );
  }
  process.exit(1);
}

console.log(
  `Coverage matrix view inventory matches ${layoutTypesPath} and ${adminViewsPath}: ${uniqueViews.length} views.`,
);

// ---------------------------------------------------------------------------
// Admin degraded-state inventory
//
// One row per ADMIN_VIEWS screen recording the shipped loading/empty/error/retry
// state, the component and browser-test files that prove it, and the paired
// screen spec. Every total is derived from the rows, so a hand-edited summary
// cannot drift away from the inventory it claims to summarise.
//
// The section deliberately lives after "## Gap Summary": the view matcher above
// only scans the text before that heading and would reject the component and
// spec filenames in these rows as unknown views.
// ---------------------------------------------------------------------------

const DEGRADED_STATE_HEADING = "## Admin degraded-state inventory";
const DEGRADED_STATES = ["loading", "empty", "error", "retry"];
const STATUS_VALUES = ["present", "missing", "not-applicable"];
const COMPONENT_ROOT = "ui/src/components/";
const SCREEN_SPEC_ROOT = "docs/reference/screen_specs/";
const UNMOCKED_ROOT = "ui/browser-tests-unmocked/";
const UNMOCKED_SUITES = ["smoke/", "full/"];
const INVENTORY_COLUMNS = [
  "screen",
  "component",
  "requires",
  ...DEGRADED_STATES,
  "evidence",
  "screenSpec",
  "unmockedProof",
];

const inventoryErrors = [];

function inventoryError(message) {
  inventoryErrors.push(`Coverage matrix degraded-state inventory: ${message}`);
}

function unquote(cell) {
  const match = cell.match(/^`(.*)`$/);
  return match ? match[1] : cell;
}

function degradedStateSection(source) {
  const occurrences = source.split(DEGRADED_STATE_HEADING).length - 1;
  if (occurrences === 0) {
    inventoryError(`${matrixPath} is missing the "${DEGRADED_STATE_HEADING}" section`);
    return null;
  }
  if (occurrences > 1) {
    inventoryError(`${matrixPath} declares the "${DEGRADED_STATE_HEADING}" section ${occurrences} times`);
    return null;
  }

  const body = source.slice(source.indexOf(DEGRADED_STATE_HEADING) + DEGRADED_STATE_HEADING.length);
  const next = body.search(/^## /m);
  return next < 0 ? body : body.slice(0, next);
}

// The section also documents the status vocabulary in a lead-in table, so rows
// are recognised by the inventory's own column count rather than by position.
function parseInventoryRows(section) {
  return section
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line.startsWith("|") && line.endsWith("|"))
    .map((line) => line.slice(1, -1).split("|").map((cell) => cell.trim()))
    .filter((cells) => cells.length === INVENTORY_COLUMNS.length)
    .filter((cells) => unquote(cells[0]) !== "Screen" && !/^:?-+:?$/.test(cells[0]))
    .map((cells) => Object.fromEntries(INVENTORY_COLUMNS.map((name, index) => [name, cells[index]])));
}

// SCREEN_REGISTRY declares one screen per line, so a same-line lookup keeps the
// capability owner in registry.ts instead of re-listing gated screens here.
function parseScreenCapabilities(source) {
  const capabilities = new Map();
  for (const line of source.split("\n")) {
    const id = line.match(/\bid:\s*"([^"]+)"/);
    if (!id) {
      continue;
    }
    const requires = line.match(/\brequires:\s*"([^"]+)"/);
    capabilities.set(id[1], requires ? requires[1] : "none");
  }
  return capabilities;
}

const lineCounts = new Map();

function fileLineCount(path) {
  if (!lineCounts.has(path)) {
    lineCounts.set(path, fs.readFileSync(path, "utf8").split("\n").length);
  }
  return lineCounts.get(path);
}

function parseEvidence(screen, cell) {
  const evidence = new Map();
  if (unquote(cell) === "none") {
    return evidence;
  }

  for (const entry of cell.split(";").map((part) => part.trim()).filter(Boolean)) {
    const match = unquote(entry).match(/^([a-z]+)=(\S+):L(\d+)$/);
    if (!match) {
      inventoryError(`${screen} evidence entry "${entry}" is not <state>=<path>:L<line>`);
      continue;
    }
    const [, state, path, line] = match;
    if (!DEGRADED_STATES.includes(state)) {
      inventoryError(`${screen} evidence names unknown state "${state}"`);
      continue;
    }
    if (evidence.has(state)) {
      inventoryError(`${screen} has more than one ${state} evidence entry`);
      continue;
    }
    evidence.set(state, { path: COMPONENT_ROOT + path, line: Number(line) });
  }
  return evidence;
}

function validateEvidence(screen, row, evidence) {
  for (const state of DEGRADED_STATES) {
    const entry = evidence.get(state);
    if (row[state] !== "present") {
      if (entry) {
        inventoryError(`${screen} has evidence for ${state} but its status is "${row[state]}"`);
      }
      continue;
    }
    if (!entry) {
      inventoryError(`${screen} is missing evidence for ${state}`);
      continue;
    }
    if (!fs.existsSync(entry.path)) {
      inventoryError(`${screen} ${state} evidence not found: ${entry.path}`);
      continue;
    }
    if (entry.line > fileLineCount(entry.path)) {
      inventoryError(`${screen} ${state} evidence line ${entry.line} is past the end of ${entry.path}`);
    }
  }
}

function validateUnmockedProof(screen, row, cell) {
  if (unquote(cell) === "none") {
    return 0;
  }

  let proofs = 0;
  for (const entry of cell.split(";").map((part) => part.trim()).filter(Boolean)) {
    const match = unquote(entry).match(/^([a-z]+)=(\S+)$/);
    if (!match) {
      inventoryError(`${screen} unmocked proof entry "${entry}" is not <state>=<path>`);
      continue;
    }
    const [, state, path] = match;
    if (!DEGRADED_STATES.includes(state)) {
      inventoryError(`${screen} unmocked proof names unknown state "${state}"`);
      continue;
    }
    proofs += 1;
    if (row[state] !== "present") {
      inventoryError(`${screen} ${state} unmocked proof requires ${state} status "present", got "${row[state]}"`);
    }
    if (!UNMOCKED_SUITES.some((suite) => path.startsWith(suite)) || !path.endsWith(".spec.ts")) {
      inventoryError(`${screen} ${state} unmocked proof must be a smoke/ or full/ spec, got ${path}`);
      continue;
    }
    if (!fs.existsSync(UNMOCKED_ROOT + path)) {
      inventoryError(`${screen} ${state} unmocked proof not found: ${UNMOCKED_ROOT + path}`);
    }
  }
  return proofs;
}

function validateInventoryRow(row, capabilities, totals) {
  const screen = unquote(row.screen);

  const component = COMPONENT_ROOT + unquote(row.component);
  if (!fs.existsSync(component)) {
    inventoryError(`${screen} component not found: ${component}`);
  }

  const declared = capabilities.get(screen);
  if (declared !== undefined && unquote(row.requires) !== declared) {
    inventoryError(`${screen} requires is "${unquote(row.requires)}" but the registry declares "${declared}"`);
  }

  for (const state of DEGRADED_STATES) {
    if (!STATUS_VALUES.includes(row[state])) {
      inventoryError(
        `${screen} ${state} status is "${row[state]}" (expected ${STATUS_VALUES.join(", ")})`,
      );
      continue;
    }
    totals[state][row[state]] += 1;
  }
  if (row.error === "not-applicable" && row.retry !== "not-applicable") {
    inventoryError(`${screen} retry status must be "not-applicable" when error is "not-applicable"`);
  }

  validateEvidence(screen, row, parseEvidence(screen, row.evidence));

  const screenSpec = unquote(row.screenSpec);
  if (screenSpec === "none") {
    totals.screenSpec.missing += 1;
  } else if (!fs.existsSync(SCREEN_SPEC_ROOT + screenSpec)) {
    inventoryError(`${screen} screen spec not found: ${SCREEN_SPEC_ROOT + screenSpec}`);
  } else {
    totals.screenSpec.present += 1;
  }

  const proofs = validateUnmockedProof(screen, row, row.unmockedProof);
  totals.unmockedProof.proofs += proofs;
  totals.unmockedProof.screens += proofs > 0 ? 1 : 0;
}

function emptyTotals() {
  const totals = {
    screenSpec: { present: 0, missing: 0 },
    unmockedProof: { proofs: 0, screens: 0 },
  };
  for (const state of DEGRADED_STATES) {
    totals[state] = Object.fromEntries(STATUS_VALUES.map((value) => [value, 0]));
  }
  return totals;
}

function reportInventory(rowCount, screenCount, totals) {
  console.log(`DEGRADED_STATE_INVENTORY:${rowCount}/${screenCount}`);
  for (const state of DEGRADED_STATES) {
    const counts = STATUS_VALUES.map((value) => `${value}=${totals[state][value]}`).join(" ");
    console.log(`DEGRADED_STATE_${state.toUpperCase()}:${counts}`);
  }
  console.log(
    `DEGRADED_STATE_SCREEN_SPEC:present=${totals.screenSpec.present} missing=${totals.screenSpec.missing}`,
  );
  console.log(
    `DEGRADED_STATE_UNMOCKED_PROOF:proofs=${totals.unmockedProof.proofs} screens=${totals.unmockedProof.screens}`,
  );
}

const adminScreens = parseStringLiteralArray(adminViewsSource, "ADMIN_VIEWS");
const section = degradedStateSection(matrixSource);
const inventoryRows = section === null ? [] : parseInventoryRows(section);
const totals = emptyTotals();
const capabilities = parseScreenCapabilities(adminViewsSource);

const seen = new Set();
const duplicated = [];
for (const row of inventoryRows) {
  const screen = unquote(row.screen);
  if (seen.has(screen)) {
    duplicated.push(screen);
  }
  seen.add(screen);
  validateInventoryRow(row, capabilities, totals);
}

if (section !== null) {
  const missingScreens = adminScreens.filter((screen) => !seen.has(screen));
  const unknownScreens = [...seen].filter((screen) => !adminScreens.includes(screen));
  if (missingScreens.length > 0) {
    inventoryError(`missing degraded-state rows for registry screens: ${missingScreens.join(", ")}`);
  }
  if (unknownScreens.length > 0) {
    inventoryError(`degraded-state rows for unknown screens: ${unknownScreens.join(", ")}`);
  }
  if (duplicated.length > 0) {
    inventoryError(`duplicate degraded-state rows: ${duplicated.join(", ")}`);
  }
}

if (inventoryErrors.length > 0) {
  for (const message of inventoryErrors) {
    console.error(message);
  }
  process.exit(1);
}

reportInventory(inventoryRows.length, adminScreens.length, totals);
NODE

echo "Coverage matrix summary from $MATRIX_PATH"
echo "Smoke = none: $smoke_none_count"
echo "Smoke = heading-only: $smoke_heading_only_count"
echo "CRUD-capable views missing full lifecycle: $crud_missing_full_count"
echo "Views missing mocked coverage: $mocked_coverage_missing_count"

if (( smoke_none_count > 0 || smoke_heading_only_count > 0 || crud_missing_full_count > 0 )); then
  echo "Coverage matrix gate failed: browser coverage gaps remain." >&2
  exit 1
fi

echo "Coverage matrix gate passed."
