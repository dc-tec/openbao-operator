#!/usr/bin/env bash
set -euo pipefail

ARTIFACT_DIR="${ARTIFACT_DIR:-artifacts}"
KIND_LOGS_DIR="${KIND_LOGS_DIR:-${ARTIFACT_DIR}/kind-logs}"

mkdir -p "${KIND_LOGS_DIR}"

if ! command -v kind >/dev/null 2>&1; then
  echo "kind not found; skipping artifact collection"
  exit 0
fi

clusters="$(kind get clusters 2>/dev/null || true)"
if [[ -z "${clusters}" ]]; then
  echo "No kind clusters found; skipping kind log export"
  exit 0
fi

echo "Exporting kind logs into ${KIND_LOGS_DIR}..."
while IFS= read -r cluster; do
  [[ -z "${cluster}" ]] && continue
  echo "- kind cluster: ${cluster}"

  mkdir -p "${KIND_LOGS_DIR}/${cluster}"

  # Export kind logs (includes node/container logs, kubelet logs, etc.)
  kind export logs "${KIND_LOGS_DIR}/${cluster}" --name "${cluster}" || true

  # Best-effort kubectl snapshots for faster diagnosis.
  ctx="kind-${cluster}"
  kubectl --context "${ctx}" get nodes -o wide >"${KIND_LOGS_DIR}/${cluster}/nodes.txt" 2>&1 || true
  kubectl --context "${ctx}" get pods -A -o wide >"${KIND_LOGS_DIR}/${cluster}/pods.txt" 2>&1 || true
  kubectl --context "${ctx}" get events -A --sort-by=.lastTimestamp >"${KIND_LOGS_DIR}/${cluster}/events.txt" 2>&1 || true
  kubectl --context "${ctx}" -n openbao-operator-system get all -o wide >"${KIND_LOGS_DIR}/${cluster}/operator-resources.txt" 2>&1 || true
  kubectl --context "${ctx}" -n openbao-operator-system logs -l app.kubernetes.io/name=openbao-operator --all-containers --tail=-1 >"${KIND_LOGS_DIR}/${cluster}/operator-logs.txt" 2>&1 || true
done <<<"${clusters}"

echo "Artifact collection complete."

