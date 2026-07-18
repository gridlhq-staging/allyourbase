#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: check-release-lane-salvage.sh [--base <ref>] [--followups <file>]... <sha> [<sha>...]
EOF
}

is_full_hex_sha() {
  local value="$1"
  [[ "$value" =~ ^[[:xdigit:]]{40}$ ]]
}

followup_names_sha() {
  local sha="$1"
  shift

  local followups_file token
  for followups_file in "$@"; do
    [[ -f "$followups_file" ]] || continue

    # Extract maximal hex runs, then require a 12+ char token that is itself a
    # prefix of the full SHA. Seven-char short SHAs collide too easily to pin
    # identity, so the floor here is twelve.
    while IFS= read -r token; do
      [[ -z "$token" ]] && continue
      if [[ "$sha" == "$token"* ]]; then
        return 0
      fi
    done < <(grep -Eo '[[:xdigit:]]+' "$followups_file" | awk 'length($0) >= 12')
  done

  return 1
}

main() {
  local base="origin/main"
  local -a followups_files=()
  local -a shas=()
  local arg

  while (($# > 0)); do
    case "$1" in
      --base)
        if (($# < 2)); then
          usage >&2
          return 2
        fi
        base="$2"
        shift 2
        ;;
      --followups)
        if (($# < 2)); then
          usage >&2
          return 2
        fi
        followups_files+=("$2")
        shift 2
        ;;
      --help)
        usage
        return 0
        ;;
      --*)
        usage >&2
        return 2
        ;;
      *)
        shas+=("$1")
        shift
        ;;
    esac
  done

  # An empty list is not a pass condition. A release lane with zero recorded
  # gate fixes must make an explicit sentinel decision instead of silently
  # treating "no input" as compliance.
  if ((${#shas[@]} == 0)); then
    usage >&2
    return 2
  fi

  local sha canonical_sha orphaned=0
  for sha in "${shas[@]}"; do
    if ! is_full_hex_sha "$sha"; then
      usage >&2
      return 2
    fi

    canonical_sha="$sha"
    # An unresolvable SHA is still a valid orphan candidate: if the commit died
    # with a pruned worktree, the follow-up file is exactly the salvage owner.
    if canonical_sha="$(git rev-parse --verify "${sha}^{commit}" 2>/dev/null)"; then
      if git merge-base --is-ancestor "$canonical_sha" "$base"; then
        continue
      fi
    fi

    if ((${#followups_files[@]} > 0)) && followup_names_sha "$sha" "${followups_files[@]}"; then
      continue
    fi

    echo "ORPHANED-FIX: $sha is neither an ancestor of $base nor named in a follow-up"
    orphaned=1
  done

  return "$orphaned"
}

main "$@"
