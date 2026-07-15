#!/usr/bin/env bash

pick_free_port() {
  local candidate
  for candidate in "$@"; do
    if ! lsof -ti :"$candidate" >/dev/null 2>&1; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done
  return 1
}
