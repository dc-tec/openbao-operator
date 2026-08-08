#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
version="$(tr -d '[:space:]' < "${repo_root}/.hugo-version")"
install_dir="${1:-bin}"

if [[ "${version}" != "0.164.0" ]]; then
  echo "unsupported Hugo version ${version}; update the pinned checksums in install-hugo.sh" >&2
  exit 1
fi

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "install-hugo.sh supports Linux CI runners; use Nix or an existing Hugo ${version} binary locally." >&2
  exit 1
fi

case "$(uname -m)" in
  x86_64)
    architecture="amd64"
    expected_sha256="fea17b8c076f950bb2e9f9486667bdaa29422883888d509d63931c73e8a9b3a4"
    ;;
  aarch64|arm64)
    architecture="arm64"
    expected_sha256="232d3bc2d1d9510625985ff7c89767598ffea5bc6e5e2882c791313f5a43f723"
    ;;
  *)
    echo "unsupported Linux architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

archive="hugo_extended_${version}_linux-${architecture}.tar.gz"
url="https://github.com/gohugoio/hugo/releases/download/v${version}/${archive}"
temporary_dir="$(mktemp -d)"
trap 'rm -rf "${temporary_dir}"' EXIT

curl --fail --location --proto '=https' --tlsv1.2 --output "${temporary_dir}/${archive}" "${url}"
printf '%s  %s\n' "${expected_sha256}" "${temporary_dir}/${archive}" | sha256sum --check --status

mkdir -p "${install_dir}"
tar -xzf "${temporary_dir}/${archive}" -C "${temporary_dir}" hugo
install -m 0755 "${temporary_dir}/hugo" "${install_dir}/hugo"
"${install_dir}/hugo" version
