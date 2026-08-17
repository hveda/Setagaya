#!/usr/bin/env bash
# Close a phase: run the full bar, then merge the working branch into the
# target branch -- the merge IS the check, so a phase cannot land without it.
#
# Three phases (10-12) merged locally to develop below the 90% coverage
# constraint, because nothing enforced the bar at merge time; the develop-
# >main promote PR was the first check, by which point the code already sat
# on develop. This script exists so that failure mode requires deliberately
# bypassing it, not simply forgetting a step.
#
# Usage: PHASE="phase 15 governance closeout" ./scripts/phase-merge.sh
#    or: ./scripts/phase-merge.sh "phase 15 governance closeout"
# Override with:
#   WORK_BRANCH=feat/honryu     # branch being closed (default: feat/honryu)
#   TARGET_BRANCH=develop       # branch merged into (default: develop)
#   COVERAGE_THRESHOLD=90       # passed through to scripts/coverage.sh
#   PHASE_MERGE_DRY_RUN=1       # run every check, stop before the merge
set -euo pipefail

cd "$(dirname "$0")/.."

PHASE="${PHASE:-${1:-}}"
if [ -z "${PHASE}" ]; then
  echo "usage: PHASE=\"phase N slug\" $0   (or: $0 \"phase N slug\")" >&2
  exit 1
fi

WORK_BRANCH="${WORK_BRANCH:-feat/honryu}"
TARGET_BRANCH="${TARGET_BRANCH:-develop}"
DRY_RUN="${PHASE_MERGE_DRY_RUN:-0}"

fail() {
  echo "REFUSED: $1" >&2
  exit 1
}

# Refuse everything cheap and structural before running anything expensive.

if [ -n "$(git status --porcelain)" ]; then
  fail "working tree is not clean -- commit or stash before closing a phase"
fi

current_branch="$(git rev-parse --abbrev-ref HEAD)"
if [ "${current_branch}" != "${WORK_BRANCH}" ]; then
  fail "on branch '${current_branch}', want '${WORK_BRANCH}' (set WORK_BRANCH to override)"
fi

if [ ! -f web/dist/.gitkeep ]; then
  fail "web/dist/.gitkeep is missing -- a staged deletion breaks 'go:embed all:dist' in fresh checkouts (phase 13 AC4)"
fi

if ! git show-ref --verify --quiet "refs/heads/${TARGET_BRANCH}"; then
  fail "target branch '${TARGET_BRANCH}' does not exist locally"
fi

echo "phase:          ${PHASE}"
echo "work branch:    ${WORK_BRANCH}"
echo "target branch:  ${TARGET_BRANCH}"
echo

echo "-- gofmt --"
unformatted="$(gofmt -l .)"
if [ -n "${unformatted}" ]; then
  echo "${unformatted}" >&2
  fail "gofmt found unformatted files (listed above)"
fi
echo "clean"

echo "-- go vet --"
go vet ./...

echo "-- golangci-lint --"
golangci-lint run

echo "-- make test --"
make test

echo "-- coverage gate --"
./scripts/coverage.sh

echo
if [ "${DRY_RUN}" = "1" ]; then
  echo "DRY RUN: every check passed; stopping before the merge."
  exit 0
fi

git checkout "${TARGET_BRANCH}"
git merge --no-ff "${WORK_BRANCH}" -m "chore: merge ${WORK_BRANCH} (${PHASE}) into ${TARGET_BRANCH}"

merge_commit="$(git rev-parse --short HEAD)"
echo
echo "merged: ${merge_commit} on ${TARGET_BRANCH}"
echo "not pushed -- publishing is a separate, deliberate step. Run:"
echo "  git push origin ${TARGET_BRANCH}"
