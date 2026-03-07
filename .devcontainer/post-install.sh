#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL_DIR="/usr/local/bin"

KIND_VERSION="${KIND_VERSION:-v0.31.0}"
KUBECTL_VERSION="${KUBECTL_VERSION:-v1.33.4}"
HELM_VERSION="${HELM_VERSION:-v3.17.4}"
TRIVY_VERSION="${TRIVY_VERSION:-v0.58.2}"
TILT_VERSION="${TILT_VERSION:-v0.37.0}"
NODE_MAJOR="${NODE_MAJOR:-20}"

go_arch="$(go env GOARCH)"
case "${go_arch}" in
  amd64)
    linux_arch="amd64"
    helm_arch="amd64"
    trivy_arch="64bit"
    tilt_arch="x86_64"
    ;;
  arm64)
    linux_arch="arm64"
    helm_arch="arm64"
    trivy_arch="ARM64"
    tilt_arch="arm64"
    ;;
  *)
    echo "unsupported architecture: ${go_arch}" >&2
    exit 1
    ;;
esac

install_binary() {
  local source_path="$1"
  local target_name="$2"

  install -m 0755 "${source_path}" "${INSTALL_DIR}/${target_name}"
}

install_from_url() {
  local url="$1"
  local target_name="$2"
  local tmp

  tmp="$(mktemp)"
  curl -fsSL "${url}" -o "${tmp}"
  install_binary "${tmp}" "${target_name}"
  rm -f "${tmp}"
}

install_from_tarball() {
  local url="$1"
  local archive_path="$2"
  local target_name="$3"
  local tmp_dir

  tmp_dir="$(mktemp -d)"
  curl -fsSL "${url}" -o "${tmp_dir}/archive.tgz"
  tar -xzf "${tmp_dir}/archive.tgz" -C "${tmp_dir}"
  install_binary "${tmp_dir}/${archive_path}" "${target_name}"
  rm -rf "${tmp_dir}"
}

ensure_apt_packages() {
  export DEBIAN_FRONTEND=noninteractive

  apt-get update
  apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    gpg \
    jq \
    make \
    python3 \
    python3-pip \
    python3-venv \
    tar \
    unzip \
    xz-utils
}

ensure_nodejs() {
  if command -v node >/dev/null 2>&1; then
    local current_major
    current_major="$(node -p 'process.versions.node.split(".")[0]')"
    if [ "${current_major}" -ge "${NODE_MAJOR}" ]; then
      return
    fi
  fi

  install -d /etc/apt/keyrings
  curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
    | gpg --yes --dearmor -o /etc/apt/keyrings/nodesource.gpg
  printf 'deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_%s.x nodistro main\n' "${NODE_MAJOR}" \
    > /etc/apt/sources.list.d/nodesource.list

  export DEBIAN_FRONTEND=noninteractive
  apt-get update
  apt-get install -y --no-install-recommends nodejs
}

ensure_kind() {
  if command -v kind >/dev/null 2>&1 && kind version 2>/dev/null | grep -q "${KIND_VERSION}"; then
    return
  fi

  install_from_url \
    "https://github.com/kubernetes-sigs/kind/releases/download/${KIND_VERSION}/kind-linux-${linux_arch}" \
    "kind"
}

ensure_kubectl() {
  if command -v kubectl >/dev/null 2>&1 && kubectl version --client --output=yaml 2>/dev/null | grep -q "${KUBECTL_VERSION}"; then
    return
  fi

  install_from_url \
    "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${linux_arch}/kubectl" \
    "kubectl"
}

ensure_helm() {
  if command -v helm >/dev/null 2>&1 && helm version --short 2>/dev/null | grep -q "${HELM_VERSION}"; then
    return
  fi

  install_from_tarball \
    "https://get.helm.sh/helm-${HELM_VERSION}-linux-${helm_arch}.tar.gz" \
    "linux-${helm_arch}/helm" \
    "helm"
}

ensure_trivy() {
  if command -v trivy >/dev/null 2>&1 && trivy --version 2>/dev/null | grep -q "${TRIVY_VERSION#v}"; then
    return
  fi

  install_from_tarball \
    "https://github.com/aquasecurity/trivy/releases/download/${TRIVY_VERSION}/trivy_${TRIVY_VERSION#v}_Linux-${trivy_arch}.tar.gz" \
    "trivy" \
    "trivy"
}

ensure_tilt() {
  if command -v tilt >/dev/null 2>&1 && tilt version 2>/dev/null | grep -q "${TILT_VERSION}"; then
    return
  fi

  install_from_tarball \
    "https://github.com/tilt-dev/tilt/releases/download/${TILT_VERSION}/tilt.${TILT_VERSION#v}.linux.${tilt_arch}.tar.gz" \
    "tilt" \
    "tilt"
}

ensure_kind_network() {
  if ! command -v docker >/dev/null 2>&1; then
    return
  fi
  if ! docker info >/dev/null 2>&1; then
    return
  fi
  if docker network inspect kind >/dev/null 2>&1; then
    return
  fi

  docker network create -d=bridge --subnet=172.19.0.0/24 kind
}

print_versions() {
  echo "Provisioned devcontainer tools:"
  go version
  docker --version
  python3 --version
  node --version
  npm --version
  kind version
  kubectl version --client
  helm version --short
  trivy --version
  tilt version
}

ensure_apt_packages
ensure_nodejs
ensure_kind
ensure_kubectl
ensure_helm
ensure_trivy
ensure_tilt
ensure_kind_network

cd "${ROOT_DIR}"
print_versions
