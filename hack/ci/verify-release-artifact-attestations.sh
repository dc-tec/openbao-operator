#!/usr/bin/env bash

set -euo pipefail

: "${REPO:?REPO is required (owner/repo)}"
: "${VERSION:?VERSION is required}"
: "${OWNER:?OWNER is required}"
: "${CHART_DIGEST:?CHART_DIGEST is required}"

CHECKSUMS_PATH="${CHECKSUMS_PATH:-dist/checksums.txt}"
SIGNER_WORKFLOW="${SIGNER_WORKFLOW:-${REPO}/.github/workflows/release.yml}"
SOURCE_REF="${SOURCE_REF:-refs/tags/${VERSION}}"
CERT_OIDC_ISSUER="${CERT_OIDC_ISSUER:-https://token.actions.githubusercontent.com}"
MAX_ATTEMPTS="${MAX_ATTEMPTS:-10}"
RETRY_SECONDS="${RETRY_SECONDS:-6}"

if [[ ! -f "${CHECKSUMS_PATH}" ]]; then
  echo "checksums file not found: ${CHECKSUMS_PATH}" >&2
  exit 1
fi

verify_oci_subject() {
  local oci_ref="$1"
  local attempts=0

  while (( attempts < MAX_ATTEMPTS )); do
    attempts=$((attempts + 1))
    if gh attestation verify "oci://${oci_ref}" \
      --repo "${REPO}" \
      --signer-workflow "${SIGNER_WORKFLOW}" \
      --source-ref "${SOURCE_REF}" \
      --cert-oidc-issuer "${CERT_OIDC_ISSUER}" \
      --deny-self-hosted-runners >/dev/null; then
      echo "Verified attestation: ${oci_ref}"
      return 0
    fi

    if (( attempts >= MAX_ATTEMPTS )); then
      echo "Failed to verify attestation after ${MAX_ATTEMPTS} attempts: ${oci_ref}" >&2
      return 1
    fi
    sleep "${RETRY_SECONDS}"
  done
}

verify_file_subject() {
  local file_path="$1"
  local attempts=0

  while (( attempts < MAX_ATTEMPTS )); do
    attempts=$((attempts + 1))
    if gh attestation verify "${file_path}" \
      --repo "${REPO}" \
      --signer-workflow "${SIGNER_WORKFLOW}" \
      --source-ref "${SOURCE_REF}" \
      --cert-oidc-issuer "${CERT_OIDC_ISSUER}" \
      --deny-self-hosted-runners >/dev/null; then
      echo "Verified attestation: ${file_path}"
      return 0
    fi

    if (( attempts >= MAX_ATTEMPTS )); then
      echo "Failed to verify attestation after ${MAX_ATTEMPTS} attempts: ${file_path}" >&2
      return 1
    fi
    sleep "${RETRY_SECONDS}"
  done
}

verify_oci_subject "ghcr.io/${OWNER}/charts/openbao-operator@${CHART_DIGEST}"
verify_file_subject "${CHECKSUMS_PATH}"
