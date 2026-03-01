#!/usr/bin/env bash

set -euo pipefail

: "${REPO:?REPO is required (owner/repo)}"
: "${OWNER:?OWNER is required}"
: "${VERSION:?VERSION is required}"
: "${MANAGER_IMAGE:?MANAGER_IMAGE is required}"
: "${MANAGER_DIGEST:?MANAGER_DIGEST is required}"
: "${CONFIG_INIT_IMAGE:?CONFIG_INIT_IMAGE is required}"
: "${CONFIG_INIT_DIGEST:?CONFIG_INIT_DIGEST is required}"
: "${BACKUP_EXECUTOR_IMAGE:?BACKUP_EXECUTOR_IMAGE is required}"
: "${BACKUP_EXECUTOR_DIGEST:?BACKUP_EXECUTOR_DIGEST is required}"
: "${UPGRADE_EXECUTOR_IMAGE:?UPGRADE_EXECUTOR_IMAGE is required}"
: "${UPGRADE_EXECUTOR_DIGEST:?UPGRADE_EXECUTOR_DIGEST is required}"
: "${CHART_DIGEST:?CHART_DIGEST is required}"

INDEX_PATH="${INDEX_PATH:-dist/provenance-index.json}"
SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-0}"
CHECKSUMS_PATH="${CHECKSUMS_PATH:-dist/checksums.txt}"
CHECKSUMS_BUNDLE_PATH="${CHECKSUMS_BUNDLE_PATH:-dist/checksums.txt.bundle}"
INSTALL_PATH="${INSTALL_PATH:-dist/install.yaml}"
CRDS_PATH="${CRDS_PATH:-dist/crds.yaml}"
SBOM_GLOB="${SBOM_GLOB:-dist/sbom-*.spdx.json}"

GOFLAGS="${GOFLAGS:--mod=vendor}" go run ./hack/tools/provenance_index \
  -mode release \
  -index-path "${INDEX_PATH}" \
  -repo "${REPO}" \
  -owner "${OWNER}" \
  -version "${VERSION}" \
  -source-date-epoch "${SOURCE_DATE_EPOCH}" \
  -manager-image "${MANAGER_IMAGE}" \
  -manager-digest "${MANAGER_DIGEST}" \
  -config-init-image "${CONFIG_INIT_IMAGE}" \
  -config-init-digest "${CONFIG_INIT_DIGEST}" \
  -backup-executor-image "${BACKUP_EXECUTOR_IMAGE}" \
  -backup-executor-digest "${BACKUP_EXECUTOR_DIGEST}" \
  -upgrade-executor-image "${UPGRADE_EXECUTOR_IMAGE}" \
  -upgrade-executor-digest "${UPGRADE_EXECUTOR_DIGEST}" \
  -chart-digest "${CHART_DIGEST}" \
  -checksums-path "${CHECKSUMS_PATH}" \
  -checksums-bundle-path "${CHECKSUMS_BUNDLE_PATH}" \
  -install-path "${INSTALL_PATH}" \
  -crds-path "${CRDS_PATH}" \
  -sbom-glob "${SBOM_GLOB}"
