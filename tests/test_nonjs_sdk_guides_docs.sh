#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
guide_dir="${repo_dir}/docs-site/guide"
readme_doc="${repo_dir}/README.md"
kotlin_guide="${guide_dir}/kotlin-sdk.md"
swift_guide="${guide_dir}/swift-sdk.md"
python_guide="${guide_dir}/python-sdk.md"
flutter_guide="${guide_dir}/flutter-sdk.md"
react_guide="${guide_dir}/react-sdk.md"
ssr_guide="${guide_dir}/ssr-sdk.md"
javascript_guide="${guide_dir}/javascript-sdk.md"
index_guide="${repo_dir}/docs-site/index.md"
getting_started_guide="${guide_dir}/getting-started.md"
deployment_guide="${guide_dir}/deployment.md"
postgis_guide="${guide_dir}/postgis.md"
go_guide="${guide_dir}/go-sdk.md"
module_path_decision="${repo_dir}/_dev/MODULE_PATH_DECISION.md"
python_manifest="${repo_dir}/sdk_python/pyproject.toml"
dart_manifest="${repo_dir}/sdk_dart/pubspec.yaml"
audited_guides=(
  "${guide_dir}/admin-dashboard.md"
  "${guide_dir}/ai-vector.md"
  "${guide_dir}/email-templates.md"
  "${guide_dir}/email.md"
  "${guide_dir}/flutter-sdk.md"
  "${guide_dir}/go-sdk.md"
  "${guide_dir}/job-queue.md"
  "${guide_dir}/kotlin-sdk.md"
  "${guide_dir}/log-drains.md"
  "${guide_dir}/mcp.md"
  "${guide_dir}/organizations.md"
  "${guide_dir}/push-notifications.md"
  "${guide_dir}/python-sdk.md"
  "${guide_dir}/react-sdk.md"
  "${guide_dir}/security.md"
  "${guide_dir}/ssr-sdk.md"
  "${guide_dir}/swift-sdk.md"
  "${guide_dir}/javascript-sdk.md"
)

fail() {
  echo "FAIL: $1"
  exit 1
}

require_file_match() {
  local file="$1"
  local pattern="$2"
  local message="$3"

  if ! rg -q -- "$pattern" "$file"; then
    fail "$message"
  fi
}

require_text_match() {
  local text="$1"
  local pattern="$2"
  local message="$3"

  if ! printf "%s\n" "$text" | rg -q -- "$pattern"; then
    fail "$message"
  fi
}

reject_files_match() {
  local pattern="$1"
  local message="$2"
  shift 2

  if rg -q -- "$pattern" "$@"; then
    fail "$message"
  fi
}

extract_kotlin_batch_code_block() {
  awk '
    /^Batch:/ { in_batch = 1; next }
    in_batch && /^```kotlin$/ { in_code = 1; next }
    in_code && /^```$/ { exit }
    in_code { print }
  ' "$kotlin_guide"
}

batch_code_block="$(extract_kotlin_batch_code_block)"

for guide in "${audited_guides[@]}"; do
  require_file_match "$guide" "<!-- audited 2026-[0-9]{2}-[0-9]{2} -->" "$(basename "$guide") missing audited marker"
done

if rg -q "Stage [0-9]" "${audited_guides[@]}"; then
  rg -n "Stage [0-9]" "${audited_guides[@]}"
  fail "audited guides leak internal stage labels"
fi

require_text_match "$batch_code_block" "import kotlinx\.serialization\.json\.buildJsonObject" "kotlin-sdk.md batch sample missing buildJsonObject import"
require_text_match "$batch_code_block" "import kotlinx\.serialization\.json\.put" "kotlin-sdk.md batch sample missing put import"

reject_files_match "\.package\(url: \"https://github\.com/allyourbase/allyourbase\.git\"" "swift-sdk.md contains invalid repo-root SwiftPM URL example" "$swift_guide"
require_file_match "$javascript_guide" "^npm install @allyourbase/js$" "javascript-sdk.md missing published npm install command"
require_file_match "$python_guide" "Preview — install from source\. Registry publishing is tracked for GA\." "python-sdk.md missing preview-from-source posture"
require_file_match "$python_guide" "^pip install ./sdk_python$" "python-sdk.md missing local path install command"
reject_files_match "^pip install allyourbase$" "python-sdk.md still presents the unrelated PyPI package as installable" "$python_guide"
require_file_match "$python_manifest" "allyourbase-sdk" "sdk_python/pyproject.toml missing intended GA package-name note"
require_file_match "$flutter_guide" "Preview — install from source\. Registry publishing is tracked for GA\." "flutter-sdk.md missing preview-from-source posture"
require_file_match "$flutter_guide" "path: ../allyourbase/sdk_dart" "flutter-sdk.md missing local path dependency example"
reject_files_match "allyourbase: \\^0\\.1\\.0" "flutter-sdk.md still presents an unverified hosted Dart package version" "$flutter_guide"
require_file_match "$dart_manifest" "^repository: https://github\\.com/griddlehq/allyourbase$" "sdk_dart/pubspec.yaml missing canonical public repository URL"
require_file_match "$dart_manifest" "^issue_tracker: https://github\\.com/griddlehq/allyourbase/issues$" "sdk_dart/pubspec.yaml missing canonical public issue tracker URL"
require_file_match "$kotlin_guide" "Preview — install from source\. Registry publishing is tracked for GA\." "kotlin-sdk.md missing preview-from-source posture"
require_file_match "$kotlin_guide" "project\\(\":sdk_kotlin\"\\)\\.projectDir = file\\(\"\\.\\./allyourbase/sdk_kotlin\"\\)" "kotlin-sdk.md missing explicit local sdk_kotlin projectDir wiring"
require_file_match "$swift_guide" "Preview — install from source\. Registry publishing is tracked for GA\." "swift-sdk.md missing preview-from-source posture"
require_file_match "$swift_guide" "\.package\(path: \"\.\./sdk_swift\"\)" "swift-sdk.md missing local sdk_swift package path example"
require_file_match "$go_guide" "Preview - install from a local checkout\\." "go-sdk.md missing current local-checkout Go SDK posture"
require_file_match "$go_guide" "^git clone https://github\\.com/griddlehq/allyourbase\\.git$" "go-sdk.md missing public source checkout command"
require_file_match "$go_guide" "^go mod edit -replace=github\\.com/allyourbase/ayb/sdk_go=/absolute/path/to/allyourbase/sdk_go$" "go-sdk.md missing local replace command for current Go SDK module"
require_file_match "$go_guide" "^go get github\\.com/allyourbase/ayb/sdk_go$" "go-sdk.md missing canonical Go SDK module get command"
reject_files_match "go get github\\.com/griddlehq/allyourbase/sdk_go@latest" "go-sdk.md still presents the failing public Go SDK path as installable" "$go_guide"
require_file_match "$module_path_decision" "sdk_go/go.mod:1.*module github\\.com/allyourbase/ayb/sdk_go" "MODULE_PATH_DECISION.md missing unchanged Go SDK module path evidence"
require_file_match "$module_path_decision" "go get github\\.com/griddlehq/allyourbase/sdk_go@latest.*module declares its path as: github\\.com/allyourbase/ayb/sdk_go" "MODULE_PATH_DECISION.md missing failing public Go SDK path evidence"
require_file_match "$module_path_decision" "Stage 1 does not rename sdk_go/go.mod or widen the public publish surface" "MODULE_PATH_DECISION.md missing Stage 1 no-rename decision"
require_file_match "$module_path_decision" "go-import/go-source metadata.*absent" "MODULE_PATH_DECISION.md missing verified vanity metadata absence"
require_file_match "$react_guide" "Preview — install from source\. Registry publishing is tracked for GA\." "react-sdk.md missing preview-from-source posture"
require_file_match "$react_guide" "^git clone https://github\\.com/griddlehq/allyourbase\\.git$" "react-sdk.md missing public source checkout command"
require_file_match "$react_guide" "^cd allyourbase/sdk && npm install && npm run build$" "react-sdk.md missing base JS SDK build step before local React install"
require_file_match "$react_guide" "^cd \.\./sdk_react && npm install && npm run build$" "react-sdk.md missing package-local React SDK build step"
require_file_match "$react_guide" "^npm install react react-dom /absolute/path/to/allyourbase/sdk /absolute/path/to/allyourbase/sdk_react$" "react-sdk.md missing local package install command"
reject_files_match "^npm install @allyourbase/js @allyourbase/react react$" "react-sdk.md still presents unpublished npm packages as installable" "$react_guide"
reject_files_match "^npm install .*@allyourbase/react" "react-sdk.md still presents the unpublished React SDK registry package as installable" "$react_guide"
require_file_match "$ssr_guide" "Preview — install from source\. Registry publishing is tracked for GA\." "ssr-sdk.md missing preview-from-source posture"
require_file_match "$ssr_guide" "^git clone https://github\\.com/griddlehq/allyourbase\\.git$" "ssr-sdk.md missing public source checkout command"
require_file_match "$ssr_guide" "^cd allyourbase/sdk && npm install && npm run build$" "ssr-sdk.md missing base JS SDK build step before local SSR install"
require_file_match "$ssr_guide" "^cd \.\./sdk_ssr && npm install && npm run build$" "ssr-sdk.md missing package-local SSR SDK build step"
require_file_match "$ssr_guide" "^npm install /absolute/path/to/allyourbase/sdk /absolute/path/to/allyourbase/sdk_ssr$" "ssr-sdk.md missing local package install command"
reject_files_match "^npm install @allyourbase/js @allyourbase/ssr$" "ssr-sdk.md still presents unpublished npm packages as installable" "$ssr_guide"
reject_files_match "^npm install .*@allyourbase/ssr" "ssr-sdk.md still presents the unpublished SSR SDK registry package as installable" "$ssr_guide"
reject_files_match "brew install (gridlhq|griddlehq)/tap/ayb" "public docs still advertise a nonexistent Homebrew tap" "$readme_doc" "$index_guide" "$getting_started_guide"
reject_files_match "ghcr\.io/gridlhq/allyourbase" "public docs still advertise a private GHCR image" "$readme_doc" "$index_guide" "$getting_started_guide" "$deployment_guide" "$postgis_guide"
reject_files_match "https://allyourbase\\.io/install\\.sh" "public docs still advertise the retired bare-domain installer URL" "$readme_doc" "$index_guide" "$getting_started_guide" "$deployment_guide"
require_file_match "$deployment_guide" "docker build -t ayb-local" "deployment.md missing local Docker build instructions"
require_file_match "$deployment_guide" "image: ayb-local" "deployment.md missing local image tag in compose example"
require_file_match "$postgis_guide" "image: ayb-local" "postgis.md missing local image tag in compose example"

echo "PASS: SDK guide install/sample checks"
