#!/usr/bin/env bash
set -euo pipefail

ARTIFACT_DIR="${ARTIFACT_DIR:-artifacts}"
KIND_LOGS_DIR="${KIND_LOGS_DIR:-${ARTIFACT_DIR}/kind-logs}"

mkdir -p "${KIND_LOGS_DIR}"

sanitize_name() {
  local value="$1"
  printf '%s' "${value}" | tr -c 'A-Za-z0-9._-' '_'
}

collect_pod_logs() {
  local ctx="$1"
  local namespace="$2"
  local pod="$3"
  local pod_dir="$4"

  mkdir -p "${pod_dir}"

  kubectl --context "${ctx}" -n "${namespace}" describe pod "${pod}" >"${pod_dir}/describe.txt" 2>&1 || true
  kubectl --context "${ctx}" -n "${namespace}" get pod "${pod}" -o yaml >"${pod_dir}/pod.yaml" 2>&1 || true

  local containers
  containers="$(kubectl --context "${ctx}" -n "${namespace}" get pod "${pod}" \
    -o jsonpath='{range .spec.initContainers[*]}init:{.name}{"\n"}{end}{range .spec.containers[*]}container:{.name}{"\n"}{end}' 2>/dev/null || true)"

  while IFS= read -r entry; do
    [[ -z "${entry}" ]] && continue

    local kind="${entry%%:*}"
    local container="${entry#*:}"
    local safe_container
    safe_container="$(sanitize_name "${container}")"

    kubectl --context "${ctx}" -n "${namespace}" logs "${pod}" -c "${container}" --tail=-1 \
      >"${pod_dir}/${kind}-${safe_container}.log" 2>&1 || true
    kubectl --context "${ctx}" -n "${namespace}" logs "${pod}" -c "${container}" --previous --tail=-1 \
      >"${pod_dir}/${kind}-${safe_container}.previous.log" 2>&1 || true
  done <<<"${containers}"
}

collect_namespace_snapshot() {
  local ctx="$1"
  local cluster_dir="$2"
  local namespace="$3"

  local safe_namespace
  safe_namespace="$(sanitize_name "${namespace}")"
  local namespace_dir="${cluster_dir}/namespaces/${safe_namespace}"
  mkdir -p "${namespace_dir}/pods"

  kubectl --context "${ctx}" -n "${namespace}" get all,configmaps,persistentvolumeclaims,networkpolicies,endpoints,endpointslices.discovery.k8s.io \
    -o wide >"${namespace_dir}/resources-wide.txt" 2>&1 || true
  kubectl --context "${ctx}" -n "${namespace}" get openbaoclusters.openbao.org,openbaotenants.openbao.org,openbaorestores.openbao.org \
    -o yaml >"${namespace_dir}/openbao-resources.yaml" 2>&1 || true
  kubectl --context "${ctx}" -n "${namespace}" get configmaps -o yaml >"${namespace_dir}/configmaps.yaml" 2>&1 || true
  kubectl --context "${ctx}" -n "${namespace}" get secrets >"${namespace_dir}/secrets.txt" 2>&1 || true
  kubectl --context "${ctx}" -n "${namespace}" get secrets \
    -o go-template='{{range .items}}{{.metadata.name}}{{"\t"}}{{range $key, $_ := .data}}{{$key}}{{" "}}{{end}}{{"\n"}}{{end}}' \
    >"${namespace_dir}/secret-keys.txt" 2>&1 || true
  kubectl --context "${ctx}" -n "${namespace}" get events --sort-by=.lastTimestamp >"${namespace_dir}/events.txt" 2>&1 || true
  kubectl --context "${ctx}" -n "${namespace}" describe pods >"${namespace_dir}/pods-describe.txt" 2>&1 || true

  local pods
  pods="$(kubectl --context "${ctx}" -n "${namespace}" get pods -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null || true)"
  while IFS= read -r pod; do
    [[ -z "${pod}" ]] && continue

    local safe_pod
    safe_pod="$(sanitize_name "${pod}")"
    collect_pod_logs "${ctx}" "${namespace}" "${pod}" "${namespace_dir}/pods/${safe_pod}"
  done <<<"${pods}"
}

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

  namespaces="$(kubectl --context "${ctx}" get namespaces -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null || true)"
  while IFS= read -r namespace; do
    [[ -z "${namespace}" ]] && continue
    case "${namespace}" in
      e2e-*|openbao-operator-system)
        echo "  collecting namespace snapshot: ${namespace}"
        collect_namespace_snapshot "${ctx}" "${KIND_LOGS_DIR}/${cluster}" "${namespace}"
        ;;
    esac
  done <<<"${namespaces}"
done <<<"${clusters}"

echo "Artifact collection complete."
