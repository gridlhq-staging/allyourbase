#!/usr/bin/env bash
set -euo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
readonly SCAN_ROOT="${STRIP_LEAKED_PATHS_ROOT:-$REPO_ROOT}"

python3 - "$SCAN_ROOT" <<'PY'
from __future__ import annotations

import os
import re
import sys
from pathlib import Path

LEAKED_PREFIX = re.compile(r"/Users/[^/\s|]+/parallel_development/.*/allyourbase_dev/")
SKIP_DIRS = {
    ".dart_tool",
    ".git",
    ".gradle",
    ".vite",
    "__pycache__",
    "build",
    "coverage",
    "dist",
    "node_modules",
    "vendor",
}


class SyncDir:
    def __init__(self) -> None:
        self.path = ""
        self.excludes: list[str] = []


class SyncScope:
    def __init__(self) -> None:
        self.dirs: list[SyncDir] = []
        self.files: set[str] = set()

    def includes(self, relative_path: str) -> bool:
        if relative_path in self.files:
            return True
        for sync_dir in self.dirs:
            if not path_is_within_dir(relative_path, sync_dir.path):
                continue
            inner_path = relative_path.removeprefix(sync_dir.path + "/")
            if matches_any_exclude(inner_path, sync_dir.excludes):
                return False
            return True
        return False


def main() -> int:
    root = Path(sys.argv[1])
    scope = load_sync_scope(root)
    for path in iter_supported_files(root, scope):
        rewrite_file(path, path.relative_to(root).as_posix())
    print("Leaked path strip completed.")
    return 0


def load_sync_scope(root: Path) -> SyncScope:
    debbie_path = root / ".debbie.toml"
    try:
        content = debbie_path.read_text(encoding="utf-8")
    except OSError as exc:
        raise SystemExit(f"read .debbie.toml failed: {exc}") from exc
    return parse_sync_scope(content)


def parse_sync_scope(content: str) -> SyncScope:
    scope = SyncScope()
    section = ""
    current_dir: SyncDir | None = None
    array_target = ""
    for raw_line in content.splitlines():
        trimmed = raw_line.strip()
        if array_target:
            apply_array_values(scope, current_dir, array_target, trimmed)
            if "]" in trimmed:
                array_target = ""
            continue
        if trimmed == "[sync]":
            section = "sync"
            current_dir = None
            continue
        if trimmed == "[[sync.dirs]]":
            section = "sync.dirs"
            current_dir = SyncDir()
            scope.dirs.append(current_dir)
            continue
        if trimmed.startswith("["):
            section = ""
            current_dir = None
            continue
        if section == "sync" and trimmed.startswith("files"):
            array_target = parse_array_line(scope, current_dir, "files", trimmed)
        if section == "sync.dirs" and trimmed.startswith("path") and current_dir:
            current_dir.path = normalize_path(first_quoted_value(trimmed))
        if section == "sync.dirs" and trimmed.startswith("exclude"):
            array_target = parse_array_line(scope, current_dir, "exclude", trimmed)
    return scope


def parse_array_line(
    scope: SyncScope, current_dir: SyncDir | None, target: str, line: str
) -> str:
    apply_array_values(scope, current_dir, target, line)
    if "[" in line and "]" not in line:
        return target
    return ""


def apply_array_values(
    scope: SyncScope, current_dir: SyncDir | None, target: str, line: str
) -> None:
    for value in quoted_values(line):
        normalized = normalize_path(value)
        if target == "files":
            scope.files.add(normalized)
        if target == "exclude" and current_dir:
            current_dir.excludes.append(normalized)


def quoted_values(line: str) -> list[str]:
    return re.findall(r'"([^"]+)"', line)


def first_quoted_value(line: str) -> str:
    values = quoted_values(line)
    return values[0] if values else ""


def normalize_path(path: str) -> str:
    return path.removeprefix("./").rstrip("/")


def path_is_within_dir(relative_path: str, sync_dir: str) -> bool:
    return relative_path == sync_dir or relative_path.startswith(sync_dir + "/")


def matches_any_exclude(relative_path: str, excludes: list[str]) -> bool:
    import fnmatch

    for exclude in excludes:
        if relative_path == exclude or relative_path.startswith(exclude + "/"):
            return True
        if fnmatch.fnmatch(Path(relative_path).name, exclude):
            return True
    return False


def iter_supported_files(root: Path, scope: SyncScope):
    for current_root, dirs, files in os.walk(root):
        dirs[:] = [name for name in dirs if name not in SKIP_DIRS]
        for name in files:
            path = Path(current_root) / name
            relative_path = path.relative_to(root).as_posix()
            if scope.includes(relative_path) and supports_surface(relative_path):
                yield path


def supports_surface(relative_path: str) -> bool:
    return (
        Path(relative_path).name == "DIRMAP.md"
        or relative_path.endswith(".go")
        or relative_path.endswith(".ts")
        or relative_path.endswith(".tsx")
    )


def rewrite_file(path: Path, relative_path: str) -> None:
    try:
        lines = path.read_text(encoding="utf-8").splitlines(keepends=True)
    except UnicodeDecodeError:
        return
    rewritten = rewrite_lines(relative_path, lines)
    if rewritten != lines:
        path.write_text("".join(rewritten), encoding="utf-8")


def rewrite_lines(relative_path: str, lines: list[str]) -> list[str]:
    if Path(relative_path).name == "DIRMAP.md":
        return rewrite_dirmap_lines(lines)
    if relative_path.endswith(".go"):
        return rewrite_go_leading_comments(lines)
    if relative_path.endswith(".ts") or relative_path.endswith(".tsx"):
        return rewrite_typescript_leading_comments(lines)
    return lines


def rewrite_dirmap_lines(lines: list[str]) -> list[str]:
    rewritten = list(lines)
    for index, line in enumerate(lines):
        if line.lstrip().startswith("|"):
            rewritten[index] = strip_leaked_prefixes(line)
    return rewritten


def rewrite_go_leading_comments(lines: list[str]) -> list[str]:
    rewritten = list(lines)
    for index, line in enumerate(lines):
        if line.strip().startswith("package "):
            break
        rewritten[index] = strip_leaked_prefixes(line)
    return rewritten


def rewrite_typescript_leading_comments(lines: list[str]) -> list[str]:
    rewritten = list(lines)
    in_block_comment = False
    for index, line in enumerate(lines):
        trimmed = line.strip()
        if is_leading_typescript_comment(trimmed, in_block_comment):
            rewritten[index] = strip_leaked_prefixes(line)
            in_block_comment = update_block_comment_state(trimmed, in_block_comment)
            continue
        if trimmed:
            break
    return rewritten


def is_leading_typescript_comment(trimmed: str, in_block_comment: bool) -> bool:
    return in_block_comment or trimmed.startswith("//") or trimmed.startswith("/*")


def update_block_comment_state(trimmed: str, in_block_comment: bool) -> bool:
    if in_block_comment:
        return "*/" not in trimmed
    return trimmed.startswith("/*") and "*/" not in trimmed


def strip_leaked_prefixes(line: str) -> str:
    return LEAKED_PREFIX.sub("", line)


if __name__ == "__main__":
    raise SystemExit(main())
PY
