#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT_PATH="$ROOT_DIR/scripts/build-postgres.sh"
source "$ROOT_DIR/tests/bash_assert_helpers.sh"

assert_equals() {
  local expected="$1"
  local actual="$2"
  local message="$3"
  if [[ "$expected" != "$actual" ]]; then
    fail "$message (expected=$expected actual=$actual)"
  fi
}

assert_command_fails() {
  local command="$1"
  local output_file="$2"
  local message="$3"
  if bash -lc "$command" >"$output_file" 2>&1; then
    fail "$message"
  fi
}

[[ -f "$SCRIPT_PATH" ]] || fail "missing $SCRIPT_PATH"

assert_contains "$SCRIPT_PATH" "normalize_arch_token()" "normalize_arch_token helper missing"
assert_contains "$SCRIPT_PATH" "verify_linux_postgres_architecture()" "verify_linux_postgres_architecture helper missing"
assert_contains "$SCRIPT_PATH" "main()" "main() boundary missing"
assert_contains "$SCRIPT_PATH" "if [[ \"\${BASH_SOURCE[0]}\" == \"\$0\" ]]; then" "source-safe main guard missing"

for arch_input in x86_64 x86-64 amd64 aarch64 arm64; do
  case "$arch_input" in
    x86_64|x86-64|amd64) expected_arch="amd64" ;;
    aarch64|arm64) expected_arch="arm64" ;;
    *) fail "unexpected arch fixture token: $arch_input" ;;
  esac

  normalized_arch="$({ source "$SCRIPT_PATH"; normalize_arch_token "$arch_input"; })"
  assert_equals "$expected_arch" "$normalized_arch" "arch normalization mismatch for token=$arch_input"
done

unsupported_output="$(mktemp)"
assert_command_fails "source \"$SCRIPT_PATH\"; normalize_arch_token sparc64" "$unsupported_output" "unsupported arch token should fail deterministically"
assert_contains "$unsupported_output" "Unsupported architecture token" "unsupported token failure message missing"
rm -f "$unsupported_output"

fixture_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$fixture_dir"
}
trap cleanup EXIT

mkdir -p "$fixture_dir/install/bin" "$fixture_dir/fakebin"
cat > "$fixture_dir/install/bin/postgres" <<'PGEOF'
#!/usr/bin/env bash
exit 0
PGEOF
chmod +x "$fixture_dir/install/bin/postgres"

cat > "$fixture_dir/fakebin/file" <<'FILEEOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" != "-b" ]]; then
  echo "unexpected invocation" >&2
  exit 2
fi
printf "%s\n" "ELF 64-bit LSB pie executable, x86-64, version 1 (SYSV)"
FILEEOF
chmod +x "$fixture_dir/fakebin/file"

mismatch_output="$fixture_dir/mismatch.txt"
assert_command_fails "PATH=\"$fixture_dir/fakebin:\$PATH\"; source \"$SCRIPT_PATH\"; verify_linux_postgres_architecture arm64 \"$fixture_dir/install\"" "$mismatch_output" "linux arch mismatch guard must fail"
assert_contains "$mismatch_output" "Architecture mismatch" "mismatch failure banner missing"
assert_contains "$mismatch_output" "detected_token=x86-64" "mismatch output missing detected token"
assert_contains "$mismatch_output" "detected_arch=amd64" "mismatch output missing detected canonical arch"
assert_contains "$mismatch_output" "requested_arch=arm64" "mismatch output missing requested arch"

cat > "$fixture_dir/fakebin/file" <<'FILEEOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" != "-b" ]]; then
  echo "unexpected invocation" >&2
  exit 2
fi
printf "%s\n" "Mach-O 64-bit executable arm64"
FILEEOF
chmod +x "$fixture_dir/fakebin/file"

non_elf_output="$fixture_dir/non_elf.txt"
assert_command_fails "PATH=\"$fixture_dir/fakebin:\$PATH\"; source \"$SCRIPT_PATH\"; verify_linux_postgres_architecture arm64 \"$fixture_dir/install\"" "$non_elf_output" "linux guard must reject non-ELF binaries"
assert_contains "$non_elf_output" "not an ELF binary" "non-ELF failure banner missing"
assert_contains "$non_elf_output" "Mach-O 64-bit executable arm64" "non-ELF failure output missing file format details"
assert_contains "$non_elf_output" "requested_arch=arm64" "non-ELF failure output missing requested arch"

echo "PASS: build_postgres_arch_guard contract checks"
