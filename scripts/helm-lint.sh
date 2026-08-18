#!/usr/bin/env bash
# Lints the honryu chart and proves it renders against the homelab values --
# catches a broken chart before it ever reaches a cluster.
#
# No cluster access: helm template only, matching this phase's own "nothing
# applied for real until cutover" rule and coverage.sh/phase-merge.sh's
# Docker-free-where-possible idiom.
set -euo pipefail

cd "$(dirname "$0")/.."

CHART="${HELM_CHART:-deploy/chart/honryu}"
VALUES="${HELM_VALUES:-deploy/chart/honryu-homelab-values.yaml}"

echo "-- helm lint --"
helm lint "${CHART}" -f "${VALUES}"

echo "-- helm template --"
helm template honryu "${CHART}" -n honryu -f "${VALUES}" > /dev/null

echo "chart OK"
