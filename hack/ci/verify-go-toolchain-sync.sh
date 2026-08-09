#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

go_mod_version="$(awk '$1 == "go" { print $2; exit }' "${ROOT_DIR}/go.mod")"
if [ -z "${go_mod_version}" ]; then
  echo "error: failed to read Go version from go.mod" >&2
  exit 1
fi

dockerfiles=()
while IFS= read -r -d '' file; do
  if grep -qE '^[[:space:]]*FROM[[:space:]].*golang:' "${file}"; then
    dockerfiles+=("${file#"${ROOT_DIR}"/}")
  fi
done < <(
  find "${ROOT_DIR}" \
    \( -path "${ROOT_DIR}/.git" -o -path "${ROOT_DIR}/.devenv" -o -path "${ROOT_DIR}/vendor" \) -prune \
    -o -type f -name 'Dockerfile*' -print0
)

if [ "${#dockerfiles[@]}" -eq 0 ]; then
  echo "error: no Dockerfiles with golang base images found" >&2
  exit 1
fi

failed=false
for dockerfile in "${dockerfiles[@]}"; do
  from_line="$(grep -m1 -E '^[[:space:]]*FROM[[:space:]].*golang:' "${ROOT_DIR}/${dockerfile}")"
  tag="$(sed -E 's/.*golang:([^@[:space:]]+).*/\1/' <<<"${from_line}")"

  if [[ ! "${tag}" =~ ^([0-9]+\.[0-9]+(\.[0-9]+)?(rc[0-9]+)?)(-|$) ]]; then
    echo "error: ${dockerfile} uses an unrecognized golang tag: ${tag}" >&2
    failed=true
    continue
  fi

  docker_go_version="${BASH_REMATCH[1]}"
  if [ "${docker_go_version}" != "${go_mod_version}" ]; then
    echo "error: ${dockerfile} uses golang:${tag}, but go.mod declares go ${go_mod_version}" >&2
    failed=true
  fi
done

devcontainer_file=".devcontainer/devcontainer.json"
devcontainer_tag="$(sed -nE 's/.*"image"[[:space:]]*:[[:space:]]*"golang:([^"@]+).*/\1/p' "${ROOT_DIR}/${devcontainer_file}" | head -n 1)"
if [ -z "${devcontainer_tag}" ]; then
  echo "error: ${devcontainer_file} does not declare a golang image" >&2
  failed=true
elif [[ ! "${devcontainer_tag}" =~ ^([0-9]+\.[0-9]+(\.[0-9]+)?(rc[0-9]+)?)(-|$) ]]; then
  echo "error: ${devcontainer_file} uses an unrecognized golang tag: ${devcontainer_tag}" >&2
  failed=true
else
  devcontainer_go_version="${BASH_REMATCH[1]}"
  if [ "${devcontainer_go_version}" != "${go_mod_version}" ]; then
    echo "error: ${devcontainer_file} uses golang:${devcontainer_tag}, but go.mod declares go ${go_mod_version}" >&2
    failed=true
  fi
fi

if [ "${failed}" = true ]; then
  echo "Go toolchain versions must stay aligned across go.mod, Dockerfile builder images, and the devcontainer." >&2
  exit 1
fi

echo "Go toolchain sync verified: go ${go_mod_version}"
