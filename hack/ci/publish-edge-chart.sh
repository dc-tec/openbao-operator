#!/usr/bin/env bash
set -euo pipefail

: "${OWNER:?OWNER is required}"
: "${CHART_VERSION:?CHART_VERSION is required}"
: "${GITHUB_OUTPUT:?GITHUB_OUTPUT is required}"

# Only this channel repository is a valid destination. Never push to the release chart repository.
[[ "${OWNER}" =~ ^[a-z0-9-]+$ ]] || { echo 'invalid registry owner' >&2; exit 1; }
[[ "${CHART_VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+-edge\.[1-9][0-9]*\.[1-9][0-9]*\.g[0-9a-f]{12}$ ]] || {
  echo 'expected a unique edge chart version' >&2
  exit 1
}
chart_ref="ghcr.io/${OWNER}/charts-edge/openbao-operator"
chart_name="openbao-operator-${CHART_VERSION}.tgz"
chart_package="dist/${chart_name}"
test -f "${chart_package}"
(
  cd dist
  awk -v name="${chart_name}" '$2 == name {found=1} END {exit !found}' checksums.txt
  sha256sum --check checksums.txt
)

work_dir="$(mktemp -d)"
trap 'rm -rf "${work_dir}"' EXIT
if helm pull "oci://${chart_ref}" --version "${CHART_VERSION}" --destination "${work_dir}" 2>"${work_dir}/pull.err"; then
  # A publisher rerun may reuse an existing version only when the package bytes match.
  cmp "${chart_package}" "${work_dir}/${chart_name}"
else
  if ! grep -Eq 'not found|MANIFEST_UNKNOWN|NAME_UNKNOWN' "${work_dir}/pull.err"; then
    cat "${work_dir}/pull.err" >&2
    exit 1
  fi
  # Retain the complete candidate image set for as long as its chart is available.
  for prefix in MANAGER CONFIG_INIT BACKUP_EXECUTOR UPGRADE_EXECUTOR; do
    image_var="${prefix}_IMAGE"
    digest_var="${prefix}_DIGEST"
    image="${!image_var}"
    digest="${!digest_var}"
    [[ "${digest}" =~ ^sha256:[0-9a-f]{64}$ ]] || { echo "invalid ${prefix} digest" >&2; exit 1; }
    docker buildx imagetools create --prefer-index=false \
      -t "${image}:edge-chart-${CHART_VERSION}" "${image}@${digest}"
  done
  helm push "${chart_package}" "oci://ghcr.io/${OWNER}/charts-edge"
fi
chart_digest="$(docker buildx imagetools inspect "${chart_ref}:${CHART_VERSION}" --format '{{json .Manifest.Digest}}' | tr -d '"')"
[[ "${chart_digest}" =~ ^sha256:[0-9a-f]{64}$ ]] || { echo 'invalid chart digest' >&2; exit 1; }
# Check that the OCI artifact contains the verified package, including after a first publication.
helm pull "oci://${chart_ref}@${chart_digest}" --destination "${work_dir}"
cmp "${chart_package}" "${work_dir}/${chart_name}"
echo "chart_digest=${chart_digest}" >> "${GITHUB_OUTPUT}"
