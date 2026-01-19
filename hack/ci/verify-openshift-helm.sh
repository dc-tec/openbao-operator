#!/usr/bin/env bash
set -euo pipefail

chart_dir="${1:-charts/openbao-operator}"
release_name="${2:-openbao-operator}"
namespace="${3:-openbao-operator-system}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_cmd helm
require_cmd csplit
require_cmd grep

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

render_and_assert() {
  local platform="$1"
  local expect_pinned="$2" # "true" or "false"
  local out="$tmpdir/rendered_${platform}.yaml"

  helm template "$release_name" "$chart_dir" \
    --namespace "$namespace" \
    --include-crds \
    --set "platform=${platform}" \
    >"$out"

  csplit -s -f "$tmpdir/doc_${platform}_" -b "%03d.yaml" "$out" '/^---$/' '{*}' >/dev/null || true

  local names=("openbao-operator-controller" "openbao-operator-provisioner")
  local found_any=0

  for name in "${names[@]}"; do
    local found=0
    for doc in "$tmpdir"/doc_"${platform}"_*.yaml; do
      [ -e "$doc" ] || continue
      if grep -qx 'kind: Deployment' "$doc" && grep -qx "  name: ${name}" "$doc"; then
        found=1
        found_any=1
        if grep -nE '(^|[[:space:]])(runAsUser:|runAsGroup:|fsGroup:)' "$doc" >/dev/null 2>&1; then
          if [ "$expect_pinned" = "false" ]; then
            echo "helm(${platform}): Deployment ${name} unexpectedly pins runAsUser/runAsGroup/fsGroup" >&2
            exit 1
          fi
        else
          if [ "$expect_pinned" = "true" ]; then
            echo "helm(${platform}): Deployment ${name} does not pin runAsUser/runAsGroup/fsGroup (expected pinned IDs on Kubernetes)" >&2
            exit 1
          fi
        fi
      fi
    done

    if [ "$found" -ne 1 ]; then
      echo "helm(${platform}): expected Deployment ${name} not found in rendered output" >&2
      exit 1
    fi
  done

  if [ "$found_any" -ne 1 ]; then
    echo "helm(${platform}): no Deployment documents found" >&2
    exit 1
  fi
}

# OpenShift mode: must NOT pin IDs (SCC injects them).
render_and_assert "openshift" "false"

# Kubernetes mode: must pin IDs (defense in depth; matches chart defaults).
render_and_assert "kubernetes" "true"

echo "OpenShift Helm regression checks passed."

