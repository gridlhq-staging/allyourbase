#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 4 ]; then
  echo "usage: $0 <markdown-file> <exact-heading> <language> <ordinal>" >&2
  exit 2
fi

markdown_file="$1"
heading="$2"
language="$3"
ordinal="$4"

if [ ! -f "$markdown_file" ]; then
  echo "file not found: $markdown_file" >&2
  exit 1
fi

case "$ordinal" in
  ''|*[!0-9]*|0)
    echo "ordinal must be a positive integer: $ordinal" >&2
    exit 2
    ;;
esac

awk -v target_heading="$heading" -v target_language="$language" -v target_ordinal="$ordinal" '
function heading_level(line, trimmed) {
  trimmed = line
  sub(/^[ \t]+/, "", trimmed)
  if (trimmed !~ /^#+[ \t]/) {
    return 0
  }
  return index(trimmed, " ") - 1
}

BEGIN {
  target_level = heading_level(target_heading)
  if (target_level == 0) {
    print "heading must include markdown # markers: " target_heading > "/dev/stderr"
    exit 2
  }
}

{
  line = $0
  sub(/\r$/, "", line)

  if (!in_section) {
    if (line == target_heading) {
      in_section = 1
      found_heading = 1
    }
    next
  }

  if (!in_fence) {
    level = heading_level(line)
    if (level > 0 && level <= target_level) {
      exit
    }
    if (line ~ /^```/) {
      fence_language = line
      sub(/^```[ \t]*/, "", fence_language)
      sub(/[ \t].*$/, "", fence_language)
      in_fence = 1
      capture = fence_language == target_language
      if (capture) {
        matching_seen++
        block = ""
      }
    }
    next
  }

  if (line ~ /^```/) {
    if (capture && matching_seen == target_ordinal) {
      printf "%s", block
      found_block = 1
      exit
    }
    in_fence = 0
    capture = 0
    next
  }

  if (capture && matching_seen == target_ordinal) {
    block = block line "\n"
  }
}

END {
  if (found_block) {
    exit 0
  }
  if (!found_heading) {
    print "heading not found: " target_heading > "/dev/stderr"
    exit 1
  }
  if (matching_seen == 0) {
    print "no fenced block with language " target_language " under heading: " target_heading > "/dev/stderr"
    exit 1
  }
  print "ordinal out of range for language " target_language " under heading: " target_heading > "/dev/stderr"
  exit 1
}
' "$markdown_file"
