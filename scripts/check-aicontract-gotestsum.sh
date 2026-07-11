#!/usr/bin/env bash
set -euo pipefail

if [[ $# -gt 1 ]]; then
  echo "Usage: $0 [gotestsum-json-path]" >&2
  exit 1
fi

readonly REPORT_PATH="${1:-/tmp/aicontract.json}"

if [[ ! -f "$REPORT_PATH" ]]; then
  echo "AI contract gotestsum results file not found: $REPORT_PATH" >&2
  exit 1
fi

# Gotestsum emits newline-delimited Go test JSON, which is intentionally kept
# separate from scripts/check-playwright-executed.sh's nested Playwright report
# parser. This script is the single owner of the AI contract required-test list.
node - "$REPORT_PATH" <<'NODE'
const fs = require("fs");

const reportPath = process.argv[2];
const requiredTests = [
  "TestOllamaContractGenerateText",
  "TestOllamaContractGenerateTextStream",
  "TestOllamaContractGenerateEmbedding",
  "TestAnthropicContractGenerateText",
  "TestAnthropicContractGenerateTextStream",
  "TestOpenAIContractGenerateText",
  "TestMoviesChatContractStreamWithRealBYOKProviders",
];

if (!reportPath) {
  console.error("Usage: check-aicontract-gotestsum.sh [gotestsum-json-path]");
  process.exit(1);
}

const states = new Map(requiredTests.map((testName) => [
  testName,
  { finalAction: "", sawSkip: false, sawTerminal: false },
]));

let lines;
try {
  lines = fs.readFileSync(reportPath, "utf8").split(/\r?\n/);
} catch (error) {
  console.error(`Failed to read gotestsum JSON report at ${reportPath}: ${error instanceof Error ? error.message : String(error)}`);
  process.exit(1);
}

for (const [index, line] of lines.entries()) {
  if (line.trim() === "") {
    continue;
  }

  let event;
  try {
    event = JSON.parse(line);
  } catch (error) {
    console.error(`Failed to parse gotestsum JSON report at ${reportPath}:${index + 1}: ${error instanceof Error ? error.message : String(error)}`);
    process.exit(1);
  }

  const testName = typeof event?.Test === "string" ? event.Test : "";
  const state = states.get(testName);
  if (!state) {
    continue;
  }

  const action = typeof event?.Action === "string" ? event.Action.toLowerCase() : "";
  if (action === "skip") {
    state.sawSkip = true;
  }
  if (action === "pass" || action === "fail" || action === "skip") {
    state.finalAction = action;
    state.sawTerminal = true;
  }
}

const failures = [];
for (const testName of requiredTests) {
  const state = states.get(testName);
  if (!state.sawTerminal) {
    failures.push(`missing ${testName}`);
    continue;
  }
  if (state.sawSkip) {
    failures.push(`skipped ${testName}`);
    continue;
  }
  if (state.finalAction !== "pass") {
    failures.push(`final status ${state.finalAction} for ${testName}`);
  }
}

if (failures.length > 0) {
  console.error(`AI contract gotestsum guard failed: ${failures.join("; ")}.`);
  process.exit(1);
}

console.log(`AI contract gotestsum guard passed: required contract tests passed: ${requiredTests.length}.`);
for (const testName of requiredTests) {
  console.log(`AI contract required test passed: ${testName}`);
}
NODE
