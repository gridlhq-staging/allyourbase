#!/usr/bin/env bash
set -euo pipefail

DEFAULT_CLOSEOUT_DIR="chats/icg"

usage() {
  cat <<'EOF'
Usage: check-closeout-repair-ancestry.sh [closeout-directory]

Checks Markdown closeout artifacts for credited repair SHAs that are not
ancestors of the same artifact's certified evidence tree.
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

  [[ -z "$PARSED_CERTIFIED_SHA" ]] || return 0

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

  if [[ "$statement" =~ ^GO[[:space:]]+on[[:space:]]+pinned[[:space:]]+evidence[[:space:]]+tree([[:space:]]+([^[:space:]]+))?$ ]]; then
    raw_certified_token="${BASH_REMATCH[2]:-}"
  else
    return 0
  fi

  if ((unmatched_emphasis_wrapper == 1)); then
    append_certified_parse_error "certified reference '${content}' has an unmatched Markdown emphasis wrapper"
    return 0
  fi

  certified_sha="$(normalize_reference_token "$raw_certified_token")"
  if ! is_commit_reference_token "$certified_sha"; then
    append_certified_parse_error "certified reference '${raw_certified_token:-<missing>}' is not a 7-40 digit hexadecimal commit token"
    return 0
  fi

  PARSED_CERTIFIED_SHA="$certified_sha"
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

parse_closeout_artifact() {
  local file="$1"
  local artifact_content section="" line content
  local verdict_first_content_seen=0

  PARSED_CERTIFIED_SHA=""
  PARSED_REPAIR_SHAS=()
  PARSED_CERTIFIED_PARSE_ERRORS=()
  PARSED_REPAIR_PARSE_ERRORS=()
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
          section="verdict"
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

    if [[ -z "$PARSED_CERTIFIED_SHA" ]]; then
      if ((${#PARSED_CERTIFIED_PARSE_ERRORS[@]} == 0)); then
        SKIPPED_ARTIFACTS=$((SKIPPED_ARTIFACTS + 1))
      fi
      continue
    fi

    ARTIFACTS_WITH_CERTIFIED_SHA=$((ARTIFACTS_WITH_CERTIFIED_SHA + 1))
    if ((${#PARSED_REPAIR_SHAS[@]} > 0)); then
      check_artifact_repairs "$file" "$PARSED_CERTIFIED_SHA" "${PARSED_REPAIR_SHAS[@]}"
    else
      check_artifact_repairs "$file" "$PARSED_CERTIFIED_SHA"
    fi
  done
}

main() {
  local closeout_dir="$DEFAULT_CLOSEOUT_DIR"

  case "$#" in
    0)
      ;;
    1)
      case "$1" in
        --help)
          usage
          return 0
          ;;
        --*)
          usage >&2
          return 2
          ;;
        *)
          closeout_dir="$1"
          ;;
      esac
      ;;
    *)
      usage >&2
      return 2
      ;;
  esac

  ARTIFACTS_SCANNED=0
  ARTIFACTS_WITH_CERTIFIED_SHA=0
  SKIPPED_ARTIFACTS=0
  REPAIR_SHAS_CHECKED=0
  VIOLATION_COUNT=0
  INDETERMINATE_COUNT=0

  scan_closeout_dir "$closeout_dir"

  if ((VIOLATION_COUNT > 0 || INDETERMINATE_COUNT > 0)); then
    printf 'SUMMARY result=FAIL artifacts_scanned=%d artifacts_with_certified_sha=%d skipped_artifacts=%d repair_shas_checked=%d violations=%d indeterminate=%d\n' \
      "$ARTIFACTS_SCANNED" "$ARTIFACTS_WITH_CERTIFIED_SHA" "$SKIPPED_ARTIFACTS" "$REPAIR_SHAS_CHECKED" "$VIOLATION_COUNT" "$INDETERMINATE_COUNT"
    return 1
  fi

  if ((ARTIFACTS_WITH_CERTIFIED_SHA == 0 || REPAIR_SHAS_CHECKED == 0)); then
    printf 'SUMMARY result=VACUOUS artifacts_scanned=%d artifacts_with_certified_sha=%d skipped_artifacts=%d repair_shas_checked=%d violations=0 indeterminate=0\n' \
      "$ARTIFACTS_SCANNED" "$ARTIFACTS_WITH_CERTIFIED_SHA" "$SKIPPED_ARTIFACTS" "$REPAIR_SHAS_CHECKED"
    return 0
  fi

  printf 'SUMMARY result=PASS artifacts_scanned=%d artifacts_with_certified_sha=%d skipped_artifacts=%d repair_shas_checked=%d violations=0 indeterminate=0\n' \
    "$ARTIFACTS_SCANNED" "$ARTIFACTS_WITH_CERTIFIED_SHA" "$SKIPPED_ARTIFACTS" "$REPAIR_SHAS_CHECKED"
}

main "$@"
