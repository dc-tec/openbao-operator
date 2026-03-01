#!/usr/bin/env bash

set -euo pipefail

: "${MANAGER_DIGEST:?MANAGER_DIGEST is required}"
: "${CONFIG_INIT_DIGEST:?CONFIG_INIT_DIGEST is required}"
: "${BACKUP_EXECUTOR_DIGEST:?BACKUP_EXECUTOR_DIGEST is required}"
: "${UPGRADE_EXECUTOR_DIGEST:?UPGRADE_EXECUTOR_DIGEST is required}"
: "${MANAGER_REBUILD_DIGEST:?MANAGER_REBUILD_DIGEST is required}"
: "${CONFIG_INIT_REBUILD_DIGEST:?CONFIG_INIT_REBUILD_DIGEST is required}"
: "${BACKUP_EXECUTOR_REBUILD_DIGEST:?BACKUP_EXECUTOR_REBUILD_DIGEST is required}"
: "${UPGRADE_EXECUTOR_REBUILD_DIGEST:?UPGRADE_EXECUTOR_REBUILD_DIGEST is required}"

PRIMARY_DIR="${PRIMARY_DIR:-dist/primary}"
REBUILD_DIR="${REBUILD_DIR:-dist/rebuild}"
REPRO_REQUIRED_FILES="${REPRO_REQUIRED_FILES:-install.yaml crds.yaml checksums.txt}"
REPRO_OPTIONAL_FILES="${REPRO_OPTIONAL_FILES:-}"

if [[ ! -d "${PRIMARY_DIR}" ]]; then
  echo "primary artifact directory not found: ${PRIMARY_DIR}" >&2
  exit 1
fi
if [[ ! -d "${REBUILD_DIR}" ]]; then
  echo "rebuild artifact directory not found: ${REBUILD_DIR}" >&2
  exit 1
fi

status=0

compare_digest() {
  local label="$1"
  local primary="$2"
  local rebuild="$3"

  if [[ "${primary}" != "${rebuild}" ]]; then
    echo "digest mismatch (${label}): primary=${primary} rebuild=${rebuild}" >&2
    status=1
    return
  fi
  echo "digest match (${label}): ${primary}"
}

compare_file() {
  local rel="$1"
  local allow_missing="$2"
  local primary_path="${PRIMARY_DIR}/${rel}"
  local rebuild_path="${REBUILD_DIR}/${rel}"

  if [[ ! -f "${primary_path}" || ! -f "${rebuild_path}" ]]; then
    if [[ "${allow_missing}" == "true" ]]; then
      echo "skipping optional file (missing in one or both dirs): ${rel}"
      return
    fi
    echo "required file missing for reproducibility check: ${rel}" >&2
    status=1
    return
  fi

  local primary_sha rebuild_sha
  primary_sha="$(sha256sum "${primary_path}" | awk '{print $1}')"
  rebuild_sha="$(sha256sum "${rebuild_path}" | awk '{print $1}')"

  if [[ "${primary_sha}" != "${rebuild_sha}" ]]; then
    echo "byte mismatch (${rel}): primary=${primary_sha} rebuild=${rebuild_sha}" >&2
    status=1
    return
  fi
  echo "byte match (${rel}): ${primary_sha}"
}

compare_digest "openbao-operator" "${MANAGER_DIGEST}" "${MANAGER_REBUILD_DIGEST}"
compare_digest "openbao-init" "${CONFIG_INIT_DIGEST}" "${CONFIG_INIT_REBUILD_DIGEST}"
compare_digest "openbao-backup" "${BACKUP_EXECUTOR_DIGEST}" "${BACKUP_EXECUTOR_REBUILD_DIGEST}"
compare_digest "openbao-upgrade" "${UPGRADE_EXECUTOR_DIGEST}" "${UPGRADE_EXECUTOR_REBUILD_DIGEST}"

for rel in ${REPRO_REQUIRED_FILES}; do
  compare_file "${rel}" "false"
done

for rel in ${REPRO_OPTIONAL_FILES}; do
  compare_file "${rel}" "true"
done

if (( status != 0 )); then
  echo "byte reproducibility verification failed" >&2
  exit 1
fi

echo "byte reproducibility verification passed"
