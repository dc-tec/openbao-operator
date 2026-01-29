#!/usr/bin/env bash
set -euo pipefail

# Helm chart "in-cluster" smoke test.
#
# Validates that:
# - the Helm chart installs into a fresh Kind cluster
# - operator deployments become Available
#
# This is intentionally lightweight compared to the full Go E2E suite.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

KIND_BIN="${KIND:-kind}"
KUBECTL_BIN="${KUBECTL:-kubectl}"
HELM_BIN="${HELM:-helm}"
DOCKER_BIN="${DOCKER:-docker}"

CLUSTER_NAME="${HELM_E2E_KIND_CLUSTER:-openbao-operator-helm-e2e}"
NAMESPACE="${HELM_E2E_NAMESPACE:-openbao-operator-system}"
KEEP_CLUSTER="${HELM_E2E_KEEP_CLUSTER:-false}"

# Use a stable local tag; override if you want to test a specific image.
OPERATOR_IMAGE="${HELM_E2E_OPERATOR_IMAGE:-example.com/openbao-operator:helm-e2e}"

if ! command -v "${KIND_BIN}" >/dev/null 2>&1; then
  echo "kind is required (set KIND or install kind)" >&2
  exit 1
fi
if ! command -v "${KUBECTL_BIN}" >/dev/null 2>&1; then
  echo "kubectl is required (set KUBECTL or install kubectl)" >&2
  exit 1
fi
if ! command -v "${HELM_BIN}" >/dev/null 2>&1; then
  echo "helm is required (set HELM or install helm)" >&2
  exit 1
fi
if ! command -v "${DOCKER_BIN}" >/dev/null 2>&1; then
  echo "docker is required (set DOCKER or install docker)" >&2
  exit 1
fi

cleanup() {
  if [[ "${KEEP_CLUSTER}" == "true" ]]; then
    echo "HELM_E2E_KEEP_CLUSTER=true: keeping Kind cluster ${CLUSTER_NAME}" >&2
    return
  fi
  "${KIND_BIN}" delete cluster --name "${CLUSTER_NAME}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

if "${KIND_BIN}" get clusters | grep -qx "${CLUSTER_NAME}"; then
  echo "Kind cluster ${CLUSTER_NAME} already exists; reusing" >&2
else
  echo "Creating Kind cluster ${CLUSTER_NAME}..." >&2
  "${KIND_BIN}" create cluster --name "${CLUSTER_NAME}" >/dev/null
fi

KUBECONFIG_PATH="$(mktemp -t openbao-operator-helm-e2e-kubeconfig.XXXXXX)"
trap 'rm -f "${KUBECONFIG_PATH}"' RETURN
export KUBECONFIG="${KUBECONFIG_PATH}"
${KIND_BIN} export kubeconfig --name "${CLUSTER_NAME}" --kubeconfig "${KUBECONFIG_PATH}" >/dev/null

echo "Syncing Helm chart from config/..." >&2
cd "${ROOT_DIR}"
make helm-sync >/dev/null

echo "Building operator image ${OPERATOR_IMAGE}..." >&2
make docker-build IMG="${OPERATOR_IMAGE}" >/dev/null

echo "Loading operator image into Kind..." >&2
${KIND_BIN} load docker-image --name "${CLUSTER_NAME}" "${OPERATOR_IMAGE}" >/dev/null

repo="${OPERATOR_IMAGE%:*}"
tag="${OPERATOR_IMAGE##*:}"

echo "Installing Helm chart into namespace ${NAMESPACE}..." >&2
${HELM_BIN} upgrade --install openbao-operator "${ROOT_DIR}/charts/openbao-operator" \
  --namespace "${NAMESPACE}" \
  --create-namespace \
  --set "image.repository=${repo}" \
  --set "image.tag=${tag}" \
  --wait \
  --timeout 5m >/dev/null

echo "Waiting for operator deployments to become Available..." >&2
${KUBECTL_BIN} wait \
  --for=condition=Available \
  deployment \
  -l app.kubernetes.io/name=openbao-operator \
  -n "${NAMESPACE}" \
  --timeout=5m >/dev/null

echo "Helm e2e smoke test succeeded." >&2

