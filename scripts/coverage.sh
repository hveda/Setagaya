#!/usr/bin/env bash
# Measure test coverage over the v3 production packages and enforce a threshold.
#
# By default this runs the full suite (unit + integration + e2e), which needs
# Docker for the testcontainers-backed adapter and e2e tests. Override with:
#   COVERAGE_TAGS=""            # unit only (no Docker)
#   COVERAGE_THRESHOLD=90       # required percentage
set -euo pipefail

cd "$(dirname "$0")/.."

THRESHOLD="${COVERAGE_THRESHOLD:-90}"
TAGS="${COVERAGE_TAGS-integration e2e}"
PROFILE="${COVERAGE_PROFILE:-cover.out}"

# Production packages only: exclude the shared conformance suites under
# internal/ports (they are test support, named <port>test), the e2e harness, the
# embed-only migrations package, and the interface-only ports package.
#
# The suites are matched by their naming convention rather than listed, so a new
# port's conformance suite is excluded the day it is written instead of quietly
# counting against the threshold.
COVERPKG=$(go list ./... \
  | grep -vE '/internal/ports/[a-z]+test$|/test/dbtest$|/test/e2e$|/migrations$|/internal/ports$' \
  | paste -sd,)

echo "coverage tags:      '${TAGS}'"
echo "coverage threshold: ${THRESHOLD}%"

# Container-backed tests must not run in parallel on a constrained host.
GOFLAGS_P=""
if [ -n "${TAGS}" ]; then
  GOFLAGS_P="-p 1"
fi

# shellcheck disable=SC2086
go test ${GOFLAGS_P} -count=1 -tags="${TAGS}" \
  -covermode=atomic -coverpkg="${COVERPKG}" -coverprofile="${PROFILE}" ./...

total=$(go tool cover -func="${PROFILE}" | awk '/^total:/ {print substr($3, 1, length($3)-1)}')
echo "total coverage: ${total}%"

awk -v total="${total}" -v thr="${THRESHOLD}" 'BEGIN {
  if (total + 0 < thr + 0) {
    printf "FAIL: coverage %.1f%% is below threshold %s%%\n", total, thr
    exit 1
  }
  printf "PASS: coverage %.1f%% meets threshold %s%%\n", total, thr
}'
