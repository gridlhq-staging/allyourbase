#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
EXTRACTOR="$REPO_DIR/scripts/extract_doc_block.sh"
DOC_ROOT="${AYB_QUICKSTART_DOC_ROOT:-$REPO_DIR}"
INVENTORY="${AYB_QUICKSTART_INVENTORY:-$REPO_DIR/tests/quickstart_doc_blocks.tsv}"
EXPECTED_INSTALL_URL="${AYB_QUICKSTART_EXPECTED_INSTALL_URL:-https://install.allyourbase.io/install.sh}"
CORPUS_FILES=(
  "README.md"
  "docs-site/guide/getting-started.md"
  "docs-site/guide/quickstart.md"
)

# shellcheck source=tests/bash_assert_helpers.sh
source "$SCRIPT_DIR/bash_assert_helpers.sh"

assert_equals() {
  local actual="$1"
  local expected="$2"
  local message="$3"
  if [ "$actual" != "$expected" ]; then
    printf 'Expected:\n%s\nActual:\n%s\n' "$expected" "$actual" >&2
    fail "$message"
  fi
}

assert_fails_with() {
  local expected="$1"
  shift
  local stderr_file
  stderr_file="$(mktemp)"
  if "$@" > /dev/null 2>"$stderr_file"; then
    cat "$stderr_file" >&2
    rm -f "$stderr_file"
    fail "command unexpectedly succeeded: $*"
  fi
  assert_contains "$stderr_file" "$expected" "stderr missing expected text: $expected"
  rm -f "$stderr_file"
}

readme_quickstart_expected="curl -fsSLo /tmp/ayb-install.sh $EXPECTED_INSTALL_URL
sh /tmp/ayb-install.sh
~/.ayb/bin/ayb start
~/.ayb/bin/ayb demo live-polls"

quickstart_create_expected='ayb sql "CREATE TABLE todos (
  id SERIAL PRIMARY KEY,
  title TEXT NOT NULL,
  completed BOOLEAN DEFAULT false,
  created_at TIMESTAMPTZ DEFAULT now()
)"'

quickstart_app_expected='import { AYBClient } from "@allyourbase/js";

const ayb = new AYBClient("http://127.0.0.1:8090");
const summarize = (todos) =>
  todos.map(({ id, title, completed }) => ({ id, title, completed }));

// Create todos
await ayb.records.create("todos", { title: "Buy groceries" });
await ayb.records.create("todos", { title: "Write docs", completed: true });
await ayb.records.create("todos", { title: "Ship v1" });

// List all todos
const { items: all } = await ayb.records.list("todos", {
  sort: "-created_at",
});
console.log("All todos:", JSON.stringify(summarize(all)));

// Filter: only incomplete
const { items: pending } = await ayb.records.list("todos", {
  filter: "completed=false",
  sort: "-created_at",
});
console.log("Pending:", JSON.stringify(summarize(pending)));

// Update: mark one as done
const todo = pending[0];
await ayb.records.update("todos", String(todo.id), { completed: true });
console.log(`Marked "${todo.title}" as done`);

// Delete
await ayb.records.delete("todos", String(todo.id));
console.log(`Deleted "${todo.title}"`);

// Final state
const { items: final } = await ayb.records.list("todos", {
  sort: "-created_at",
});
console.log("Remaining:", JSON.stringify(summarize(final)));'

assert_equals "$("$EXTRACTOR" "$DOC_ROOT/README.md" '## Quickstart' bash 1)" "$readme_quickstart_expected" "README quickstart block mismatch"
assert_equals "$("$EXTRACTOR" "$DOC_ROOT/docs-site/guide/quickstart.md" '## 2. Create a todos table' bash 1)" "$quickstart_create_expected" "quickstart create table block mismatch"
assert_equals "$("$EXTRACTOR" "$DOC_ROOT/docs-site/guide/quickstart.md" '## 4. Write the app' js 1)" "$quickstart_app_expected" "quickstart app block mismatch"

assert_fails_with "file not found" "$EXTRACTOR" "$REPO_DIR/nope.md" '## Quickstart' bash 1
assert_fails_with "heading not found" "$EXTRACTOR" "$REPO_DIR/README.md" '## Missing' bash 1
assert_fails_with "no fenced block" "$EXTRACTOR" "$REPO_DIR/README.md" '## Quickstart' sql 1
assert_fails_with "ordinal out of range" "$EXTRACTOR" "$REPO_DIR/README.md" '## Quickstart' bash 2

enumerate_corpus_fences() {
  awk -v doc_root="$DOC_ROOT" '
    FNR == 1 {
      if (NR > 1 && in_fence) {
        printf "unterminated fenced block in %s\n", previous_file > "/dev/stderr"
        errors = 1
      }
      relative_file = substr(FILENAME, length(doc_root) + 2)
      previous_file = relative_file
      heading = ""
      in_fence = 0
    }
    {
      line = $0
      sub(/\r$/, "", line)

      if (in_fence) {
        if (line ~ /^[[:space:]]*```/) {
          in_fence = 0
        }
        next
      }

      if (line ~ /^#+[[:space:]]/) {
        heading = line
        next
      }

      if (line ~ /^[[:space:]]*```/) {
        language = line
        sub(/^[[:space:]]*```[[:space:]]*/, "", language)
        sub(/[[:space:]].*$/, "", language)
        locator_key = relative_file SUBSEP heading SUBSEP language
        ordinal = ++ordinals[locator_key]
        printf "%s\t%s\t%s\t%d\n", relative_file, heading, language, ordinal
        in_fence = 1
      }
    }
    END {
      if (in_fence) {
        printf "unterminated fenced block in %s\n", previous_file > "/dev/stderr"
        errors = 1
      }
      exit errors
    }
  ' "${CORPUS_FILES[@]/#/$DOC_ROOT/}"
}

reconcile_inventory_with_corpus() {
  local discovered_file="$1"
  awk -F '\t' '
    function is_runnable(language, normalized) {
      normalized = tolower(language)
      return normalized ~ /^(bash|sh|shell|sql|javascript|js|typescript|ts)$/
    }
    function report(message) {
      print message > "/dev/stderr"
      errors = 1
    }
    NR == FNR {
      if (FNR == 1) {
        next
      }
      if (NF != 9) {
        report("inventory row must have 9 fields: line " FNR " has " NF)
        next
      }
      if (ids[$1]++) {
        report("duplicate inventory id: " $1)
      }
      if ($6 == "url") {
        if ($8 != "covered") {
          report("URL inventory status must be covered on line " FNR ": " $8)
        }
        next
      }
      if ($6 != "fence") {
        report("invalid inventory locator type on line " FNR ": " $6)
        next
      }

      locator = $2 "\t" $3 "\t" $4 "\t" $5
      if (inventory_locators[locator]++) {
        report("duplicate inventory locator: " $2 " | " $3 " | " $4 " | " $5)
      }
      inventory_files[locator] = $2
      inventory_headings[locator] = $3
      inventory_languages[locator] = $4
      inventory_ordinals[locator] = $5

      if (is_runnable($4)) {
        runnable_total++
        if ($8 == "covered") {
          covered++
          if ($7 !~ /^(tests\/test_quickstart_e2e\.sh|tests\/test_install\.sh|_dev\/manual_smoke_tests\/17_demo_launch\.test\.sh)$/) {
            report("covered runnable fence has invalid owner on line " FNR ": " $7)
          }
        } else if ($8 == "allowlisted") {
          if ($9 == "") {
            report("allowlisted runnable fence needs a reason on line " FNR)
          }
          allowlisted_count++
          allowlisted_locators[allowlisted_count] = $2 " | " $3 " | " $4 " | " $5
          allowlisted_reasons[allowlisted_count] = $9
        } else {
          report("invalid runnable inventory status on line " FNR ": " $8)
        }
      } else {
        if ($8 != "excluded") {
          report("non-runnable fence must be excluded on line " FNR ": " $8)
        }
        if ($9 == "") {
          report("excluded non-runnable fence needs a reason on line " FNR)
        }
      }
      next
    }
    {
      if (NF != 4) {
        report("discovered fence must have 4 fields: line " FNR " has " NF)
        next
      }
      locator = $1 "\t" $2 "\t" $3 "\t" $4
      if (discovered_locators[locator]++) {
        report("duplicate discovered fence: " $1 " | " $2 " | " $3 " | " $4)
      }
      if (!(locator in inventory_locators)) {
        classification = is_runnable($3) ? "runnable" : "non-runnable"
        report("missing " classification " inventory locator: " $1 " | " $2 " | " $3 " | " $4)
      }
    }
    END {
      for (locator in inventory_locators) {
        if (!(locator in discovered_locators)) {
          report("stale inventory locator: " inventory_files[locator] " | " inventory_headings[locator] " | " inventory_languages[locator] " | " inventory_ordinals[locator])
        }
      }
      printf "covered=%d/%d\n", covered, runnable_total
      for (allowlist_index = 1; allowlist_index <= allowlisted_count; allowlist_index++) {
        printf "allowlisted: %s -- %s\n", allowlisted_locators[allowlist_index], allowlisted_reasons[allowlist_index]
      }
      exit errors
    }
  ' "$INVENTORY" "$discovered_file"
}

validate_coverage_gate() {
  local discovered_file
  discovered_file="$(mktemp)"
  if ! enumerate_corpus_fences >"$discovered_file"; then
    rm -f "$discovered_file"
    return 1
  fi
  if ! reconcile_inventory_with_corpus "$discovered_file"; then
    rm -f "$discovered_file"
    return 1
  fi
  rm -f "$discovered_file"
}

validate_coverage_gate

while IFS=$'\t' read -r id file heading language ordinal locator_type owner status notes; do
  [ "$id" = "id" ] && continue
  [ "$locator_type" = "fence" ] || continue
  "$EXTRACTOR" "$DOC_ROOT/$file" "$heading" "$language" "$ordinal" >/dev/null || {
    fail "inventory locator does not extract: $id"
  }
done < "$INVENTORY"

assert_uninventoried_runnable_fence_fails_closed() {
  local scratch_dir
  local stderr_file
  scratch_dir="$(mktemp -d)"
  stderr_file="$scratch_dir/stderr"

  mkdir -p "$scratch_dir/docs-site/guide" "$scratch_dir/tests"
  cp "$REPO_DIR/README.md" "$scratch_dir/README.md"
  cp "$REPO_DIR/docs-site/guide/getting-started.md" "$scratch_dir/docs-site/guide/getting-started.md"
  cp "$REPO_DIR/docs-site/guide/quickstart.md" "$scratch_dir/docs-site/guide/quickstart.md"
  cp "$INVENTORY" "$scratch_dir/tests/quickstart_doc_blocks.tsv"

  cat >>"$scratch_dir/docs-site/guide/quickstart.md" <<'EOF'

## Stage 5 Scratch Drift Probe

```bash
ayb stage-five-uninventoried-command
```
EOF

  if AYB_QUICKSTART_DOC_ROOT="$scratch_dir" \
    AYB_QUICKSTART_INVENTORY="$scratch_dir/tests/quickstart_doc_blocks.tsv" \
    QUICKSTART_SKIP_DRIFT_REGRESSION=1 \
    bash "$0" >/dev/null 2>"$stderr_file"; then
    rm -rf "$scratch_dir"
    fail "uninventoried runnable fence unexpectedly passed coverage gate"
  fi

  if ! grep -Fq "docs-site/guide/quickstart.md" "$stderr_file" \
    || ! grep -Fq "## Stage 5 Scratch Drift Probe" "$stderr_file"; then
    cat "$stderr_file" >&2
  fi
  assert_contains "$stderr_file" "docs-site/guide/quickstart.md" "coverage failure missing scratch document path"
  assert_contains "$stderr_file" "## Stage 5 Scratch Drift Probe" "coverage failure missing scratch heading"
  rm -rf "$scratch_dir"
}

if [ "${QUICKSTART_SKIP_DRIFT_REGRESSION:-0}" != "1" ]; then
  assert_uninventoried_runnable_fence_fails_closed
fi

echo "PASS: doc block extractor contract succeeded"
