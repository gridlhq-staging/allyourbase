#!/usr/bin/env bash
# Run Playwright E2E tests and upload results to S3.
# Usage: ./e2e/run-and-upload.sh [playwright args...]
#
# Requires:
#   AWS CLI configured
#   AYB_ADMIN_PASSWORD set
#   MAILPAIL_DOMAIN and MAILPAIL_BUCKET set (for email tests)
#
# Results uploaded to: s3://shareborough-e2e/runs/<timestamp>/

set -euo pipefail

cd "$(dirname "$0")/.."

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
S3_BUCKET="shareborough-e2e"
S3_PREFIX="runs/${TIMESTAMP}"

echo "=== E2E Test Run: ${TIMESTAMP} ==="
echo "Results will be uploaded to s3://${S3_BUCKET}/${S3_PREFIX}/"

# Ensure admin password is set
if [ -z "${AYB_ADMIN_PASSWORD:-}" ]; then
  echo "ERROR: AYB_ADMIN_PASSWORD not set. Export it before running."
  exit 1
fi

# Warn if mailpail not configured (email tests will be skipped)
if [ -z "${MAILPAIL_DOMAIN:-}" ] || [ -z "${MAILPAIL_BUCKET:-}" ]; then
  echo "WARNING: MAILPAIL_DOMAIN or MAILPAIL_BUCKET not set. Email tests will be skipped."
fi

# Ensure dependencies are installed
npm install --silent 2>/dev/null || true
npx playwright install chromium --with-deps 2>/dev/null || true

mkdir -p test-results

# Run tests (continue even if tests fail — we want to upload results)
set +e
npx playwright test "$@" --reporter=list,html 2>&1 | tee "test-results/run-${TIMESTAMP}.log"
TEST_EXIT=$?
set -e

echo ""
echo "=== Tests finished with exit code: ${TEST_EXIT} ==="
echo "=== Uploading results to S3 ==="

# Upload HTML report
if [ -d "playwright-report" ]; then
  aws s3 sync playwright-report/ "s3://${S3_BUCKET}/${S3_PREFIX}/report/" --quiet
  echo "  HTML report: s3://${S3_BUCKET}/${S3_PREFIX}/report/"
fi

# Upload test results (screenshots, traces)
if [ -d "test-results" ]; then
  aws s3 sync test-results/ "s3://${S3_BUCKET}/${S3_PREFIX}/results/" --quiet
  echo "  Test results: s3://${S3_BUCKET}/${S3_PREFIX}/results/"
fi

# Upload a summary file
echo "{\"timestamp\":\"${TIMESTAMP}\",\"exitCode\":${TEST_EXIT}}" > "/tmp/run-summary.json"
aws s3 cp "/tmp/run-summary.json" "s3://${S3_BUCKET}/${S3_PREFIX}/summary.json" --quiet

echo ""
echo "=== Upload complete ==="
echo "View report: aws s3 sync s3://${S3_BUCKET}/${S3_PREFIX}/report/ ./report-${TIMESTAMP}/"
echo "Exit code: ${TEST_EXIT}"

exit ${TEST_EXIT}
