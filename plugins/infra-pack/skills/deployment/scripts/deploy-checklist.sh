#!/bin/bash
# Pre-deploy validation checklist
# Usage: ./deploy-checklist.sh <image-name> <image-tag> <chart-path> [namespace]

set -euo pipefail

IMAGE="${1:?Usage: deploy-checklist.sh <image-name> <image-tag> <chart-path> [namespace]}"
TAG="${2:?Tag required}"
CHART="${3:?Chart path required}"
NAMESPACE="${4:-${APP_NAMESPACE:?Pass a namespace as arg 4 or set APP_NAMESPACE}}"
REGISTRY="${IMAGE_REGISTRY:?Set IMAGE_REGISTRY env var (e.g. <region>-docker.pkg.dev/<gcp-project>/<registry-repo>)}"

echo "=== Pre-Deploy Checklist ==="
echo "Image: $REGISTRY/$IMAGE:$TAG"
echo "Chart: $CHART"
echo ""

PASS=0
FAIL=0

# check <description> <command> [args...] — runs the command directly (argv
# array, no `eval`/shell re-parse), so interpolated image/chart/namespace
# values can never be re-interpreted as shell syntax.
check() {
  local desc="$1"; shift
  if "$@" > /dev/null 2>&1; then
    echo "  PASS: $desc"
    ((PASS++))
  else
    echo "  FAIL: $desc"
    ((FAIL++))
  fi
}

# 1. Image exists in registry
echo "--- Image Checks ---"
check "Image exists in registry" gcloud artifacts docker images describe "$REGISTRY/$IMAGE:$TAG"

# 2. Chart validation
echo ""
echo "--- Chart Checks ---"
check "Chart.yaml exists" test -f "$CHART/Chart.yaml"
check "Helm lint passes" helm lint "$CHART"

# 3. Cluster connectivity
echo ""
echo "--- Cluster Checks ---"
check "kubectl connected" kubectl cluster-info
check "Namespace exists" kubectl get ns "$NAMESPACE"

# 4. Summary
echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="
[ "$FAIL" -eq 0 ] && echo "Ready to deploy!" || echo "Fix failures before deploying."
exit $FAIL
