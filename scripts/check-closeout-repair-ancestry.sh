#!/usr/bin/env bash
set -euo pipefail

DEFAULT_CLOSEOUT_DIR="chats/icg"

# Historical UNBOUND_VERDICT specimens are immutable closeouts that predate the
# pinned-evidence verdict grammar. Each row binds the expected basename, exact
# first Verdict content, and reason so the real-corpus freshness check cannot be
# satisfied by a different artifact that happens to reuse the same text.
HISTORICAL_UNBOUND_VERDICT_CARVEOUTS="$(cat <<'EOF'
jul21_6pm_demo_perfect_closeout.md|The batch closes. The falsifiable claim is now: every public demo passes the default aggregate `make demo-check`, covering screen specs, demo unit suites, desktop and mobile Chromium flows, critical/serious accessibility checks, InstantSearch, push smoke, freshness, and the live production workflow arm.|immutable history: no pinned evidence-tree certification.
jun01_pm_0_closeout.md|**READY** to cut `v0.0.9-beta`.|immutable history: release readiness without a pinned evidence-tree certification.
EOF
)"
readonly HISTORICAL_UNBOUND_VERDICT_CARVEOUTS

usage() {
  cat <<'EOF'
Usage: check-closeout-repair-ancestry.sh [--require-corpus] [closeout-directory]

Checks that certifying Markdown closeouts declare their repairs and that every
credited repair SHA is an ancestor of the same artifact's certified tree.
--require-corpus additionally rejects a missing or non-certifying corpus.
EOF
}

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s\n' "$value"
}

append_repair_sha() {
  local entry="$1"
  local raw_repair_token repair_sha repair_reference trailing_content

  [[ "$entry" =~ ^[[:space:]]*[-*][[:space:]] ]] || return 0
  if [[ "$(trim "$entry")" == "* Zero repairs: this certifying closeout credits no repair commits." ]]; then
    PARSED_HAS_REPAIR_DECLARATION=1
    return 0
  fi
  if ! repair_reference="$(extract_affirmative_repair_reference "$entry")"; then
    return 0
  fi

  repair_reference="$(trim "$repair_reference")"
  if [[ -z "$repair_reference" ]]; then
    append_repair_parse_error "repair reference '<missing>' is not a 7-40 digit hexadecimal commit token"
    return 0
  fi

  [[ "$repair_reference" =~ ^([^[:space:]]+)([[:space:]]+(.*))?$ ]]
  raw_repair_token="${BASH_REMATCH[1]}"
  trailing_content="$(trim "${BASH_REMATCH[3]:-}")"
  repair_sha="$(normalize_reference_token "$raw_repair_token")"
  if ! is_commit_reference_token "$repair_sha"; then
    append_repair_parse_error "repair reference '${raw_repair_token:-<missing>}' is not a 7-40 digit hexadecimal commit token"
    return 0
  fi
  if [[ -n "$trailing_content" ]]; then
    append_repair_parse_error "repair reference '${raw_repair_token} ${trailing_content}' continues after credited SHA"
    return 0
  fi

  PARSED_REPAIR_SHAS+=("$repair_sha")
  PARSED_HAS_REPAIR_DECLARATION=1
}

extract_affirmative_repair_reference() {
  local remaining="$1"
  local candidate prefix

  while [[ "$remaining" =~ repaired[[:space:]]+(at|on)(.*)$ ]]; do
    candidate="${BASH_REMATCH[2]}"
    prefix="${remaining%%repaired*}"

    # Ignore a negated or embedded occurrence, then continue looking: a later
    # negation must not hide an earlier or later affirmative repair credit.
    if [[ "$prefix" =~ (^|[^[:alnum:]_])not[[:space:]]+$ ]] ||
      [[ "$prefix" =~ [[:alnum:]_]$ ]]; then
      remaining="${remaining#*repaired}"
      continue
    fi

    printf '%s\n' "$candidate"
    return 0
  done

  return 1
}

append_verdict_sha() {
  local content="$1"
  local statement raw_certified_token certified_sha
  local unmatched_emphasis_wrapper=0

  if is_historical_unbound_verdict_content "$content"; then
    PARSED_VERDICT_CLASSIFICATION="UNBOUND_VERDICT"
    return 0
  fi

  statement="$content"
  if [[ "${content:0:2}" == "**" && "${content: -2}" == "**" ]]; then
    ((${#content} >= 4)) || return 0
    statement="${content:2:${#content}-4}"
  elif [[ "${content:0:2}" == "**" || "${content: -2}" == "**" ]]; then
    unmatched_emphasis_wrapper=1
    if [[ "${content:0:2}" == "**" ]]; then
      statement="${content:2}"
    else
      statement="${content:0:${#content}-2}"
    fi
  fi

  if [[ "$statement" =~ ^NO-GO([[:space:]].*)?$ ]] ||
    [[ "$statement" =~ ^GO[[:space:]]+WITHDRAWN([[:space:].].*)?$ ]]; then
    PARSED_VERDICT_CLASSIFICATION="NON_CERTIFYING"
    return 0
  fi

  if [[ "$statement" =~ ^GO[[:space:]]+on[[:space:]]+pinned[[:space:]]+evidence[[:space:]]+tree([[:space:]]+([^[:space:]]+))?$ ]]; then
    raw_certified_token="${BASH_REMATCH[2]:-}"
  else
    PARSED_VERDICT_CLASSIFICATION="UNKNOWN"
    append_certified_parse_error "UNKNOWN_VERDICT: first Verdict content '${content}' is not a recognized closeout verdict"
    return 0
  fi

  if ((unmatched_emphasis_wrapper == 1)); then
    PARSED_VERDICT_CLASSIFICATION="UNKNOWN"
    append_certified_parse_error "certified reference '${content}' has an unmatched Markdown emphasis wrapper"
    return 0
  fi

  PARSED_VERDICT_CLASSIFICATION="CERTIFYING"
  certified_sha="$(normalize_reference_token "$raw_certified_token")"
  if ! is_commit_reference_token "$certified_sha"; then
    append_certified_parse_error "certified reference '${raw_certified_token:-<missing>}' is not a 7-40 digit hexadecimal commit token"
    return 0
  fi

  PARSED_CERTIFIED_SHA="$certified_sha"
}

is_historical_unbound_verdict_content() {
  local content="$1"
  local expected_basename expected_content reason

  while IFS='|' read -r expected_basename expected_content reason; do
    [[ -n "$expected_basename" ]] || continue
    if [[ "$content" == "$expected_content" ]]; then
      return 0
    fi
  done <<<"$HISTORICAL_UNBOUND_VERDICT_CARVEOUTS"

  return 1
}

record_historical_unbound_carveout_match() {
  local file="$1"
  local content="$2"
  local basename expected_basename expected_content reason
  local index=0

  basename="$(basename -- "$file")"
  while IFS='|' read -r expected_basename expected_content reason; do
    [[ -n "$expected_basename" ]] || continue
    if [[ "$basename" == "$expected_basename" && "$content" == "$expected_content" ]]; then
      HISTORICAL_UNBOUND_VERDICT_CARVEOUT_MATCHED[index]=1
    fi
    index=$((index + 1))
  done <<<"$HISTORICAL_UNBOUND_VERDICT_CARVEOUTS"
}

report_stale_historical_unbound_carveouts() {
  local expected_basename expected_content reason
  local index=0

  STALE_CARVEOUT_COUNT=0
  while IFS='|' read -r expected_basename expected_content reason; do
    [[ -n "$expected_basename" ]] || continue
    if [[ "${HISTORICAL_UNBOUND_VERDICT_CARVEOUT_MATCHED[$index]:-0}" != "1" ]]; then
      printf 'STALE_CARVEOUT: expected historical UNBOUND_VERDICT specimen %s is absent (%s)\n' "$expected_basename" "$reason" >&2
      STALE_CARVEOUT_COUNT=$((STALE_CARVEOUT_COUNT + 1))
    fi
    index=$((index + 1))
  done <<<"$HISTORICAL_UNBOUND_VERDICT_CARVEOUTS"
}

append_certified_parse_error() {
  PARSED_CERTIFIED_PARSE_ERRORS+=("$1")
}

append_repair_parse_error() {
  PARSED_REPAIR_PARSE_ERRORS+=("$1")
}

is_commit_reference_token() {
  local token="$1"
  [[ "$token" =~ ^[[:xdigit:]]{7,40}$ ]]
}

normalize_reference_token() {
  local token="$1"
  local first last

  token="$(trim "$token")"

  case "${token: -1}" in
    ',' | '.' | ':' | ';')
      token="${token%?}"
      ;;
  esac

  while [[ -n "$token" ]]; do
    first="${token:0:1}"
    last="${token: -1}"
    case "$first" in
      '`')
        [[ "$last" == '`' ]] || break
        token="${token:1:${#token}-2}"
        ;;
      '(')
        [[ "$last" == ')' ]] || break
        token="${token:1:${#token}-2}"
        ;;
      '[')
        [[ "$last" == ']' ]] || break
        token="${token:1:${#token}-2}"
        ;;
      *)
        break
        ;;
    esac
  done

  printf '%s\n' "$token"
}

flush_repair_entry() {
  [[ -n "$CURRENT_REPAIR_ENTRY" ]] || return 0
  append_repair_sha "$CURRENT_REPAIR_ENTRY"
  CURRENT_REPAIR_ENTRY=""
}

# Stage 1 closeout-corpus measurement, recorded 2026-07-30 at
# 7bf026bf8de029823e939c51adf5a0e3604ebfdb. Sources were the stage
# denominator commands run from the repo root plus the existing parser contract
# below; this block is a Stage 2 handoff and does not define active behavior.
#
# Denominator: 346 chats/icg Markdown artifacts; 40 *closeout*.md artifacts; 7
# /^## Verdict/ prefix hits; 2 /^## Repairs/ prefix hits. Current guard output:
# PASS artifacts_scanned=346 artifacts_with_certified_sha=1
# skipped_artifacts=306 no_verdict_section=35 repair_shas_checked=11
# violations=0 indeterminate=0 stale_carveouts=0.
# The founding specimen at 22b9bc90e exits 0 with VACUOUS
# artifacts_scanned=1 artifacts_with_certified_sha=1 repair_shas_checked=0.
#
# Verdict-prefix triage table:
# - jul18_pm_oss_release_readiness_closeout.md | ## Verdicts | no Repairs |
#   "OSS release gate: GREEN for the three release-blocker proofs and latest required repository gates. Stage 1 recorded the final green full unit and managed-Postgres runs at `9eb706ff2759c8560dc40e21cd58df5f3618c067`, after the earlier red integration findings were diagnosed as repo-owned and fixed. The current Stage 3 HEAD is `4b44100e2a6664d9e711d8760f22ec7b6e6f15f3`, which contains Stage 1, Stage 2, and the L13 merge history." | UNRECOGNIZED | immutable history: plural heading is outside the exact parser contract.
# - jul20_pm_ship_and_deepen_closeout.md | ## Verdicts | no Repairs |
#   "Repository batch verdict: GREEN. The approved Stage 1 verification chain is" | UNRECOGNIZED | immutable history:
#   plural heading is outside the exact parser contract.
# - jul21_6pm_demo_perfect_closeout.md | ## Verdict | no Repairs |
#   "The batch closes. The falsifiable claim is now: every public demo passes the default aggregate `make demo-check`, covering screen specs, demo unit suites, desktop and mobile Chromium flows, critical/serious accessibility checks, InstantSearch, push smoke, freshness, and the live production workflow arm." | UNBOUND_VERDICT | immutable history: no pinned
#   evidence-tree certification.
# - jul27_9pm_launch_completion_closeout.md | ## Verdict | no Repairs |
#   "NO-GO on the all-green automation launch gate." | NON_CERTIFYING |
#   immutable history: explicit NO-GO.
# - jul28_1pm_launch_gate_green_closeout.md | ## Verdict | has Repairs |
#   "**GO WITHDRAWN. No verdict currently stands. Re-certification is pending.**" | NON_CERTIFYING |
#   immutable history: withdrawn unsound GO; the incident repairs
#   0cfda34eb, 4a148d864, and 81f0f04fa are not ancestors of fb6f291e.
# - jul29_8pm_launch_gate_truth_and_debt_closeout.md | ## Verdict |
#   has Repairs | "GO on pinned evidence tree d0fad4ae846e3f6f509919298460a4c6a9905335" | CERTIFYING |
#   active guarded specimen.
# - jun01_pm_0_closeout.md | ## Verdict | no Repairs | "**READY** to cut `v0.0.9-beta`." |
#   UNBOUND_VERDICT | immutable history: release readiness without a pinned
#   evidence-tree certification.
#
# Measured classification counts: CERTIFYING=1, NON_CERTIFYING=2,
# UNBOUND_VERDICT=2 (jul21_6pm_demo_perfect_closeout.md, jun01_pm_0_closeout.md),
# UNRECOGNIZED=2. UNBOUND_VERDICT is a surfaced Stage 2 category, separate from
# skipped_artifacts. The 35 *closeout*.md files without an exact ## Verdict
# heading are surfaced as no_verdict_section only; Stage 4 does not enforce new
# Verdict headings. L12 still carries its Stage 5 "## Repairs" instruction, so
# the older L0 gap spec is obsolete for this guard.
#
# Stage 2 future artifact contract: any future CERTIFYING closeout must include
# a ## Repairs section with at least one "- " or "* " bullet containing either a
# real repair credit or an explicit zero-repairs declaration. Satisfiable
# examples for known-answer tests:
#   ## Repairs
#   - Kanban Cancel ambiguity repaired at `0cfda34eb`
#   ## Repairs
#   * Zero repairs: this certifying closeout credits no repair commits.
parse_closeout_artifact() {
  local file="$1"
  local artifact_content section="" line content
  local verdict_first_content_seen=0

  PARSED_CERTIFIED_SHA=""
  PARSED_REPAIR_SHAS=()
  PARSED_CERTIFIED_PARSE_ERRORS=()
  PARSED_REPAIR_PARSE_ERRORS=()
  PARSED_VERDICT_CLASSIFICATION="NO_VERDICT"
  PARSED_VERDICT_CONTENT=""
  PARSED_HAS_REPAIR_DECLARATION=0
  CURRENT_REPAIR_ENTRY=""

  if ! artifact_content="$(cat -- "$file")"; then
    return 1
  fi

  # Parser contract:
  # - Only exact "## Verdict" and "## Repairs" section bodies are inspected,
  #   each bounded by the next level-two heading.
  # - A certified tree exists only when the first nonblank Verdict content is a
  #   direct "GO on pinned evidence tree <sha>" statement. "GO WITHDRAWN" and
  #   later historical mentions of an older GO are skipped.
  # - Repair SHAs are extracted only from bullet entries that explicitly credit
  #   a repair with "repaired at" or "repaired on"; unrelated hex tokens in the
  #   Repairs section are not repair credit.
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" =~ ^##[[:space:]] ]]; then
      flush_repair_entry
      case "$line" in
        "## Verdict")
          if ((verdict_first_content_seen == 0)); then
            section="verdict"
            PARSED_VERDICT_CLASSIFICATION="PENDING"
          else
            section=""
          fi
          ;;
        "## Repairs")
          section="repairs"
          ;;
        *)
          section=""
          ;;
      esac
      continue
    fi

    case "$section" in
      verdict)
        content="$(trim "$line")"
        if ((verdict_first_content_seen == 0)) && [[ -n "$content" ]]; then
          verdict_first_content_seen=1
          PARSED_VERDICT_CONTENT="$content"
          append_verdict_sha "$content"
        fi
        ;;
      repairs)
        if [[ "$line" =~ ^[[:space:]]*[-*][[:space:]] ]]; then
          flush_repair_entry
          CURRENT_REPAIR_ENTRY="$line"
        elif [[ -n "$CURRENT_REPAIR_ENTRY" ]]; then
          CURRENT_REPAIR_ENTRY+=" $line"
        fi
        ;;
    esac
  done <<<"$artifact_content"

  flush_repair_entry
  if [[ "$PARSED_VERDICT_CLASSIFICATION" == "PENDING" ]]; then
    PARSED_VERDICT_CLASSIFICATION="UNKNOWN"
    append_certified_parse_error "UNKNOWN_VERDICT: exact Verdict section has no content"
  fi
}

report_parse_errors() {
  local file="$1"
  local parse_error

  if ((${#PARSED_CERTIFIED_PARSE_ERRORS[@]} > 0)); then
    for parse_error in "${PARSED_CERTIFIED_PARSE_ERRORS[@]}"; do
      printf 'INDETERMINATE: %s in %s\n' "$parse_error" "$file" >&2
      INDETERMINATE_COUNT=$((INDETERMINATE_COUNT + 1))
    done
  fi

  if [[ -n "$PARSED_CERTIFIED_SHA" ]] && ((${#PARSED_REPAIR_PARSE_ERRORS[@]} > 0)); then
    for parse_error in "${PARSED_REPAIR_PARSE_ERRORS[@]}"; do
      printf 'INDETERMINATE: %s in %s\n' "$parse_error" "$file" >&2
      INDETERMINATE_COUNT=$((INDETERMINATE_COUNT + 1))
    done
  fi
}

resolve_commit() {
  local kind="$1"
  local sha="$2"
  local file="$3"
  local resolved

  if ! resolved="$(git rev-parse --verify "${sha}^{commit}" 2>/dev/null)"; then
    printf 'INDETERMINATE: %s SHA %s in %s is not a unique resolvable commit\n' "$kind" "$sha" "$file" >&2
    return 1
  fi

  printf '%s\n' "$resolved"
}

check_artifact_repairs() {
  local file="$1"
  local certified_sha="$2"
  shift 2
  local -a repair_shas=("$@")
  local certified_commit repair_sha repair_commit merge_base_status

  if ! certified_commit="$(resolve_commit "certified" "$certified_sha" "$file")"; then
    INDETERMINATE_COUNT=$((INDETERMINATE_COUNT + 1))
    return 0
  fi

  if (($# == 0)); then
    return 0
  fi

  for repair_sha in "${repair_shas[@]}"; do
    if ! repair_commit="$(resolve_commit "repair" "$repair_sha" "$file")"; then
      INDETERMINATE_COUNT=$((INDETERMINATE_COUNT + 1))
      continue
    fi

    REPAIR_SHAS_CHECKED=$((REPAIR_SHAS_CHECKED + 1))
    if git merge-base --is-ancestor "$repair_commit" "$certified_commit"; then
      printf 'OK: %s repair %s is an ancestor of certified tree %s\n' "$file" "$repair_commit" "$certified_commit"
    else
      merge_base_status=$?
      case "$merge_base_status" in
        1)
          printf 'VIOLATION: %s repair %s is not an ancestor of certified tree %s\n' "$file" "$repair_commit" "$certified_commit"
          VIOLATION_COUNT=$((VIOLATION_COUNT + 1))
          ;;
        *)
          printf 'INDETERMINATE: %s ancestry evaluation failed for repair %s against certified tree %s with exit status %d\n' \
            "$file" "$repair_commit" "$certified_commit" "$merge_base_status" >&2
          INDETERMINATE_COUNT=$((INDETERMINATE_COUNT + 1))
          ;;
      esac
    fi
  done
}

scan_closeout_dir() {
  local closeout_dir="$1"
  local file file_list link_check_path scan_root symlink_entries
  local -a files=()

  link_check_path="$closeout_dir"
  while [[ "$link_check_path" != "/" && "$link_check_path" == */ ]]; do
    link_check_path="${link_check_path%/}"
  done
  if [[ -L "$link_check_path" ]]; then
    printf 'INDETERMINATE: closeout path %s is a symbolic link\n' "$closeout_dir" >&2
    INDETERMINATE_COUNT=$((INDETERMINATE_COUNT + 1))
    return 0
  fi

  if [[ ! -e "$closeout_dir" ]]; then
    printf 'VACUOUS: closeout directory %s does not exist\n' "$closeout_dir"
    return 0
  fi

  if [[ ! -d "$closeout_dir" ]]; then
    printf 'INDETERMINATE: closeout path %s exists but is not a directory\n' "$closeout_dir" >&2
    INDETERMINATE_COUNT=$((INDETERMINATE_COUNT + 1))
    return 0
  fi
  CORPUS_AVAILABLE=1

  scan_root="$closeout_dir"
  case "$scan_root" in
    /*) ;;
    *) scan_root="./$scan_root" ;;
  esac

  if ! symlink_entries="$(find "$scan_root" -type l -print)"; then
    printf 'INDETERMINATE: failed to inspect symbolic links in closeout directory %s\n' "$closeout_dir" >&2
    INDETERMINATE_COUNT=$((INDETERMINATE_COUNT + 1))
    return 0
  fi
  if [[ -n "$symlink_entries" ]]; then
    printf 'INDETERMINATE: closeout directory %s contains a symbolic link\n' "$closeout_dir" >&2
    INDETERMINATE_COUNT=$((INDETERMINATE_COUNT + 1))
    return 0
  fi

  if ! file_list="$(mktemp "${TMPDIR:-/tmp}/closeout-repair-ancestry.XXXXXX")"; then
    printf 'INDETERMINATE: failed to allocate temporary file for closeout traversal\n' >&2
    INDETERMINATE_COUNT=$((INDETERMINATE_COUNT + 1))
    return 0
  fi

  if ! find "$scan_root" -type f -name '*.md' -print0 | sort -z >"$file_list"; then
    printf 'INDETERMINATE: failed to enumerate Markdown closeout artifacts in %s\n' "$closeout_dir" >&2
    INDETERMINATE_COUNT=$((INDETERMINATE_COUNT + 1))
    rm -f "$file_list"
    return 0
  fi

  while IFS= read -r -d '' file; do
    files+=("$file")
  done <"$file_list"
  rm -f "$file_list"

  for file in "${files[@]}"; do
    ARTIFACTS_SCANNED=$((ARTIFACTS_SCANNED + 1))
    if ! parse_closeout_artifact "$file"; then
      printf 'INDETERMINATE: failed to read closeout artifact %s\n' "$file" >&2
      INDETERMINATE_COUNT=$((INDETERMINATE_COUNT + 1))
      continue
    fi
    report_parse_errors "$file"

    case "$PARSED_VERDICT_CLASSIFICATION" in
      NO_VERDICT)
        case "$(basename -- "$file")" in
          *closeout*.md)
            NO_VERDICT_SECTION_COUNT=$((NO_VERDICT_SECTION_COUNT + 1))
            ;;
          *)
            SKIPPED_ARTIFACTS=$((SKIPPED_ARTIFACTS + 1))
            ;;
        esac
        continue
        ;;
      NON_CERTIFYING)
        NON_CERTIFYING_COUNT=$((NON_CERTIFYING_COUNT + 1))
        continue
        ;;
      UNBOUND_VERDICT)
        UNBOUND_VERDICT_COUNT=$((UNBOUND_VERDICT_COUNT + 1))
        record_historical_unbound_carveout_match "$file" "$PARSED_VERDICT_CONTENT"
        continue
        ;;
      UNKNOWN)
        continue
        ;;
      CERTIFYING)
        [[ -n "$PARSED_CERTIFIED_SHA" ]] || continue
        ;;
    esac

    ARTIFACTS_WITH_CERTIFIED_SHA=$((ARTIFACTS_WITH_CERTIFIED_SHA + 1))
    if ((PARSED_HAS_REPAIR_DECLARATION == 0)); then
      printf 'UNCREDITED: certifying artifact %s has no qualifying ## Repairs declaration\n' "$file" >&2
      UNCREDITED_COUNT=$((UNCREDITED_COUNT + 1))
      continue
    fi

    if ((${#PARSED_REPAIR_SHAS[@]} > 0)); then
      check_artifact_repairs "$file" "$PARSED_CERTIFIED_SHA" "${PARSED_REPAIR_SHAS[@]}"
    else
      check_artifact_repairs "$file" "$PARSED_CERTIFIED_SHA"
    fi
  done
}

print_summary() {
  local result="$1"
  printf 'SUMMARY result=%s artifacts_scanned=%d artifacts_with_certified_sha=%d skipped_artifacts=%d no_verdict_section=%d non_certifying=%d unbound_verdict=%d uncredited=%d repair_shas_checked=%d violations=%d indeterminate=%d stale_carveouts=%d\n' \
    "$result" "$ARTIFACTS_SCANNED" "$ARTIFACTS_WITH_CERTIFIED_SHA" "$SKIPPED_ARTIFACTS" "$NO_VERDICT_SECTION_COUNT" \
    "$NON_CERTIFYING_COUNT" "$UNBOUND_VERDICT_COUNT" "$UNCREDITED_COUNT" "$REPAIR_SHAS_CHECKED" "$VIOLATION_COUNT" \
    "$INDETERMINATE_COUNT" "$STALE_CARVEOUT_COUNT"
}

main() {
  local closeout_dir="$DEFAULT_CLOSEOUT_DIR"
  local require_corpus=0
  local using_default_closeout_dir=1
  local corpus_requirement_failed=0

  case "$#" in
    0) ;;
    1)
      case "$1" in
        --help)
          usage
          return 0
          ;;
        --require-corpus)
          require_corpus=1
          ;;
        --*)
          usage >&2
          return 2
          ;;
        *)
          closeout_dir="$1"
          using_default_closeout_dir=0
          ;;
      esac
      ;;
    2)
      if [[ "$1" != "--require-corpus" || "$2" == --* ]]; then
        usage >&2
        return 2
      fi
      require_corpus=1
      closeout_dir="$2"
      using_default_closeout_dir=0
      ;;
    *)
      usage >&2
      return 2
      ;;
  esac

  ARTIFACTS_SCANNED=0
  ARTIFACTS_WITH_CERTIFIED_SHA=0
  SKIPPED_ARTIFACTS=0
  NO_VERDICT_SECTION_COUNT=0
  NON_CERTIFYING_COUNT=0
  UNBOUND_VERDICT_COUNT=0
  HISTORICAL_UNBOUND_VERDICT_CARVEOUT_MATCHED=()
  UNCREDITED_COUNT=0
  REPAIR_SHAS_CHECKED=0
  VIOLATION_COUNT=0
  INDETERMINATE_COUNT=0
  STALE_CARVEOUT_COUNT=0
  CORPUS_AVAILABLE=0

  scan_closeout_dir "$closeout_dir"

  if ((require_corpus == 1)); then
    if ((CORPUS_AVAILABLE == 0)); then
      printf 'REQUIRED_CORPUS: closeout corpus %s is unavailable\n' "$closeout_dir" >&2
      corpus_requirement_failed=1
    elif ((ARTIFACTS_WITH_CERTIFIED_SHA == 0)); then
      printf 'REQUIRED_CORPUS: closeout corpus %s has zero certifying artifacts\n' "$closeout_dir" >&2
      corpus_requirement_failed=1
    fi
    if ((CORPUS_AVAILABLE == 1 && using_default_closeout_dir == 1 && UNBOUND_VERDICT_COUNT != 2)); then
      printf 'REQUIRED_CORPUS: default corpus unbound_verdict=%d, want measured value 2\n' "$UNBOUND_VERDICT_COUNT" >&2
      corpus_requirement_failed=1
    fi
    if ((CORPUS_AVAILABLE == 1 && using_default_closeout_dir == 1)); then
      report_stale_historical_unbound_carveouts
      if ((STALE_CARVEOUT_COUNT > 0)); then
        corpus_requirement_failed=1
      fi
    fi
    # Do not pin no_verdict_section; that would impose an undeclared ## Verdict contract on L12 and future closeout authors.
  fi

  if ((VIOLATION_COUNT > 0 || INDETERMINATE_COUNT > 0 || UNCREDITED_COUNT > 0 || corpus_requirement_failed > 0)); then
    print_summary "FAIL"
    return 1
  fi

  if ((ARTIFACTS_WITH_CERTIFIED_SHA == 0)); then
    print_summary "VACUOUS"
    return 0
  fi

  print_summary "PASS"
}

main "$@"
