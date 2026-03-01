#!/usr/bin/env bash

set -euo pipefail

: "${CHART_VERSION:?CHART_VERSION is required}"
: "${OWNER:?OWNER is required}"

CHART_DIR="${CHART_DIR:-charts/openbao-operator}"
CHART_FILE="${CHART_DIR}/Chart.yaml"

if [[ ! -f "${CHART_FILE}" ]]; then
  echo "chart file not found: ${CHART_FILE}" >&2
  exit 1
fi

if [[ "${CHART_VERSION}" == *-* ]]; then
  prerelease="true"
else
  prerelease="false"
fi

if ! grep -Eq '^[[:space:]]*artifacthub\.io/prerelease:' "${CHART_FILE}"; then
  echo "artifacthub.io/prerelease annotation not found in ${CHART_FILE}" >&2
  exit 1
fi

sed -E -i.bak \
  's|^([[:space:]]*artifacthub\.io/prerelease:[[:space:]]*).*$|\1"'"${prerelease}"'"|' \
  "${CHART_FILE}"
rm -f "${CHART_FILE}.bak"

if ! grep -Eq '^[[:space:]]*artifacthub\.io/images:' "${CHART_FILE}"; then
  echo "artifacthub.io/images annotation not found in ${CHART_FILE}" >&2
  exit 1
fi

awk -v owner="${OWNER}" -v version="${CHART_VERSION}" '
  BEGIN {in_images=0; replaced=0}
  /^[[:space:]]*artifacthub\.io\/images:[[:space:]]*\|[[:space:]]*$/ {
    print "  artifacthub.io/images: |"
    print "    - name: openbao-operator"
    print "      image: ghcr.io/" owner "/openbao-operator:" version
    print "    - name: openbao-init"
    print "      image: ghcr.io/" owner "/openbao-init:" version
    print "    - name: openbao-backup"
    print "      image: ghcr.io/" owner "/openbao-backup:" version
    print "    - name: openbao-upgrade"
    print "      image: ghcr.io/" owner "/openbao-upgrade:" version
    in_images=1
    replaced=1
    next
  }
  in_images {
    if ($0 ~ /^  artifacthub\.io\//) {
      in_images=0
    } else {
      next
    }
  }
  { print }
  END {
    if (replaced != 1) {
      exit 44
    }
  }
' "${CHART_FILE}" > "${CHART_FILE}.tmp" || {
  code="$?"
  rm -f "${CHART_FILE}.tmp"
  if [[ "${code}" == "44" ]]; then
    echo "failed to replace artifacthub.io/images annotation block" >&2
  fi
  exit "${code}"
}
mv "${CHART_FILE}.tmp" "${CHART_FILE}"

actual_prerelease="$(sed -nE 's/^[[:space:]]*artifacthub\.io\/prerelease:[[:space:]]*"(true|false)"[[:space:]]*$/\1/p' "${CHART_FILE}" | head -n1)"
if [[ -z "${actual_prerelease}" || "${actual_prerelease}" != "${prerelease}" ]]; then
  echo "artifacthub.io/prerelease mismatch: expected=${prerelease} actual=${actual_prerelease:-<empty>}" >&2
  exit 1
fi

images=(
  openbao-operator
  openbao-init
  openbao-backup
  openbao-upgrade
)
for image in "${images[@]}"; do
  if ! grep -Eq "^[[:space:]]*image:[[:space:]]*ghcr\\.io/${OWNER}/${image}:${CHART_VERSION}[[:space:]]*$" "${CHART_FILE}"; then
    echo "artifacthub.io/images entry missing or invalid for ${image}" >&2
    exit 1
  fi
done
