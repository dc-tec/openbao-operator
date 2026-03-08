#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 4 || $# -gt 5 ]]; then
  echo "usage: $0 <source-dir> <image> <operator-version> <output-dir> [goflags]" >&2
  exit 1
fi

source_dir="$1"
image="$2"
operator_version="$3"
output_dir="$4"
goflags="${5:--mod=vendor}"

mkdir -p "${output_dir}"

(
  cd "${source_dir}"
  GOFLAGS="${goflags}" make build-installer IMG="${image}" OPERATOR_VERSION="${operator_version}"
  cp dist/install.yaml "${output_dir}/install.yaml"
  GOFLAGS="${goflags}" make build-crds
  cp dist/crds.yaml "${output_dir}/crds.yaml"
)
