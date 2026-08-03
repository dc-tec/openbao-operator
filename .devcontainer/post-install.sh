#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL_DIR="/usr/local/bin"

KIND_VERSION="${KIND_VERSION:-v0.31.0}"
KUBECTL_VERSION="${KUBECTL_VERSION:-v1.35.1}"
HELM_VERSION="${HELM_VERSION:-v3.17.4}"
TRIVY_VERSION="${TRIVY_VERSION:-v0.69.3}"
TILT_VERSION="${TILT_VERSION:-v0.37.0}"
NODE_VERSION="${NODE_VERSION:-v$(tr -d '[:space:]' < "${ROOT_DIR}/.node-version")}"
PNPM_VERSION="${PNPM_VERSION:-$(sed -n 's/.*"packageManager": "pnpm@\([^"]*\)".*/\1/p' "${ROOT_DIR}/website/package.json" | head -n 1)}"

go_arch="$(go env GOARCH)"
case "${go_arch}" in
  amd64)
    linux_arch="amd64"
    helm_arch="amd64"
    trivy_arch="64bit"
    tilt_arch="x86_64"
    node_arch="x64"
    ;;
  arm64)
    linux_arch="arm64"
    helm_arch="arm64"
    trivy_arch="ARM64"
    tilt_arch="arm64"
    node_arch="arm64"
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

install_symlink() {
  local source_path="$1"
  local target_name="$2"

  ln -sf "${source_path}" "${INSTALL_DIR}/${target_name}"
}

download_to_file() {
  local url="$1"
  local output_path="$2"

  curl -fsSL "${url}" -o "${output_path}"
}

install_from_url() {
  local url="$1"
  local target_name="$2"
  local tmp

  tmp="$(mktemp)"
  download_to_file "${url}" "${tmp}"
  install_binary "${tmp}" "${target_name}"
  rm -f "${tmp}"
}

install_from_tarball() {
  local url="$1"
  local archive_path="$2"
  local target_name="$3"
  local tmp_dir

  tmp_dir="$(mktemp -d)"
  download_to_file "${url}" "${tmp_dir}/archive.tgz"
  tar -xzf "${tmp_dir}/archive.tgz" -C "${tmp_dir}"
  install_binary "${tmp_dir}/${archive_path}" "${target_name}"
  rm -rf "${tmp_dir}"
}

verify_sha256_from_checksums() {
  local checksums_path="$1"
  local asset_name="$2"
  local artifact_path="$3"
  local expected_sha=""

  expected_sha="$(awk -v name="${asset_name}" '
    {
      candidate=$2
      sub(/^\*/, "", candidate)
      if (candidate == name) {
        print $1
        exit
      }
    }
  ' "${checksums_path}")"

  if [[ -z "${expected_sha}" ]]; then
    echo "failed to find checksum for ${asset_name}" >&2
    exit 1
  fi

  printf '%s  %s\n' "${expected_sha}" "${artifact_path}" | sha256sum -c -
}

verify_sha256_file() {
  local checksum_path="$1"
  local artifact_path="$2"
  local expected_sha=""

  expected_sha="$(tr -d '\r\n[:space:]' < "${checksum_path}")"
  if [[ -z "${expected_sha}" ]]; then
    echo "empty checksum file ${checksum_path}" >&2
    exit 1
  fi

  printf '%s  %s\n' "${expected_sha}" "${artifact_path}" | sha256sum -c -
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
    local current_version
    current_version="$(node -p 'process.versions.node')"
    if [ "${current_version}" = "${NODE_VERSION#v}" ]; then
      return
    fi
  fi

  local asset_name="node-${NODE_VERSION}-linux-${node_arch}.tar.xz"
  local checksums_name="SHASUMS256.txt"
  local base_url="https://nodejs.org/download/release/${NODE_VERSION}"
  local install_root="/usr/local/lib/node-${NODE_VERSION}"
  local tmp_dir

  tmp_dir="$(mktemp -d)"
  download_to_file "${base_url}/${asset_name}" "${tmp_dir}/${asset_name}"
  download_to_file "${base_url}/${checksums_name}" "${tmp_dir}/${checksums_name}"
  verify_sha256_from_checksums "${tmp_dir}/${checksums_name}" "${asset_name}" "${tmp_dir}/${asset_name}"
  rm -rf "${install_root}"
  install -d /usr/local/lib
  tar -xJf "${tmp_dir}/${asset_name}" -C /usr/local/lib
  mv "/usr/local/lib/node-${NODE_VERSION}-linux-${node_arch}" "${install_root}"
  install_symlink "${install_root}/bin/node" "node"
  install_symlink "${install_root}/bin/npm" "npm"
  install_symlink "${install_root}/bin/npx" "npx"
  install_symlink "${install_root}/bin/corepack" "corepack"
  rm -rf "${tmp_dir}"
}

ensure_pnpm() {
  if command -v pnpm >/dev/null 2>&1 && [ "$(pnpm --version 2>/dev/null)" = "${PNPM_VERSION}" ]; then
    return
  fi

  if ! command -v corepack >/dev/null 2>&1; then
    echo "corepack is required to install pnpm ${PNPM_VERSION}" >&2
    exit 1
  fi

  corepack prepare "pnpm@${PNPM_VERSION}" --activate
  corepack enable pnpm --install-directory "${INSTALL_DIR}"
}

ensure_kind() {
  if command -v kind >/dev/null 2>&1 && kind version 2>/dev/null | grep -q "${KIND_VERSION}"; then
    return
  fi

  local asset_name="kind-linux-${linux_arch}"
  local base_url="https://kind.sigs.k8s.io/dl/${KIND_VERSION}"
  local tmp_dir

  tmp_dir="$(mktemp -d)"
  download_to_file "${base_url}/${asset_name}" "${tmp_dir}/${asset_name}"
  download_to_file "${base_url}/${asset_name}.sha256sum" "${tmp_dir}/${asset_name}.sha256sum"
  verify_sha256_from_checksums "${tmp_dir}/${asset_name}.sha256sum" "${asset_name}" "${tmp_dir}/${asset_name}"
  install_binary "${tmp_dir}/${asset_name}" "kind"
  rm -rf "${tmp_dir}"
}

ensure_kubectl() {
  if command -v kubectl >/dev/null 2>&1 && kubectl version --client --output=yaml 2>/dev/null | grep -q "${KUBECTL_VERSION}"; then
    return
  fi

  local base_url="https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${linux_arch}"
  local tmp_dir

  tmp_dir="$(mktemp -d)"
  download_to_file "${base_url}/kubectl" "${tmp_dir}/kubectl"
  download_to_file "${base_url}/kubectl.sha256" "${tmp_dir}/kubectl.sha256"
  verify_sha256_file "${tmp_dir}/kubectl.sha256" "${tmp_dir}/kubectl"
  install_binary "${tmp_dir}/kubectl" "kubectl"
  rm -rf "${tmp_dir}"
}

ensure_helm() {
  if command -v helm >/dev/null 2>&1 && helm version --short 2>/dev/null | grep -q "${HELM_VERSION}"; then
    return
  fi

  local asset_name="helm-${HELM_VERSION}-linux-${helm_arch}.tar.gz"
  local base_url="https://get.helm.sh"
  local tmp_dir

  tmp_dir="$(mktemp -d)"
  download_to_file "${base_url}/${asset_name}" "${tmp_dir}/${asset_name}"
  download_to_file "${base_url}/${asset_name}.sha256sum" "${tmp_dir}/${asset_name}.sha256sum"
  verify_sha256_from_checksums "${tmp_dir}/${asset_name}.sha256sum" "${asset_name}" "${tmp_dir}/${asset_name}"
  tar -xzf "${tmp_dir}/${asset_name}" -C "${tmp_dir}"
  install_binary "${tmp_dir}/linux-${helm_arch}/helm" "helm"
  rm -rf "${tmp_dir}"
}

ensure_trivy() {
  if command -v trivy >/dev/null 2>&1 && trivy --version 2>/dev/null | grep -q "${TRIVY_VERSION#v}"; then
    return
  fi

  local asset_name="trivy_${TRIVY_VERSION#v}_Linux-${trivy_arch}.tar.gz"
  local checksums_name="trivy_${TRIVY_VERSION#v}_checksums.txt"
  local base_url="https://github.com/aquasecurity/trivy/releases/download/${TRIVY_VERSION}"
  local tmp_dir

  tmp_dir="$(mktemp -d)"
  download_to_file "${base_url}/${asset_name}" "${tmp_dir}/${asset_name}"
  download_to_file "${base_url}/${checksums_name}" "${tmp_dir}/${checksums_name}"
  verify_sha256_from_checksums "${tmp_dir}/${checksums_name}" "${asset_name}" "${tmp_dir}/${asset_name}"
  tar -xzf "${tmp_dir}/${asset_name}" -C "${tmp_dir}"
  install_binary "${tmp_dir}/trivy" "trivy"
  rm -rf "${tmp_dir}"
}

ensure_tilt() {
  if command -v tilt >/dev/null 2>&1 && tilt version 2>/dev/null | grep -q "${TILT_VERSION}"; then
    return
  fi

  local asset_name="tilt.${TILT_VERSION#v}.linux.${tilt_arch}.tar.gz"
  local checksums_name="checksums.txt"
  local base_url="https://github.com/tilt-dev/tilt/releases/download/${TILT_VERSION}"
  local tmp_dir

  tmp_dir="$(mktemp -d)"
  download_to_file "${base_url}/${asset_name}" "${tmp_dir}/${asset_name}"
  download_to_file "${base_url}/${checksums_name}" "${tmp_dir}/${checksums_name}"
  verify_sha256_from_checksums "${tmp_dir}/${checksums_name}" "${asset_name}" "${tmp_dir}/${asset_name}"
  tar -xzf "${tmp_dir}/${asset_name}" -C "${tmp_dir}"
  install_binary "${tmp_dir}/tilt" "tilt"
  rm -rf "${tmp_dir}"
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
  pnpm --version
  kind version
  kubectl version --client
  helm version --short
  trivy --version
  tilt version
}

ensure_apt_packages
ensure_nodejs
ensure_pnpm
ensure_kind
ensure_kubectl
ensure_helm
ensure_trivy
ensure_tilt
ensure_kind_network

cd "${ROOT_DIR}"
print_versions
