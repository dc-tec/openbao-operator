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
# Isolate the digest pull so a prior version pull cannot satisfy the comparison.
digest_dir="${work_dir}/digest"
mkdir "${digest_dir}"
helm pull "oci://${chart_ref}@${chart_digest}" --destination "${digest_dir}"
# Helm derives the archive filename from the digest reference, not the chart version.
shopt -s nullglob
digest_archives=("${digest_dir}"/*.tgz)
if [[ "${#digest_archives[@]}" -ne 1 ]]; then
  echo "expected exactly one chart archive from digest pull, found ${#digest_archives[@]}" >&2
  exit 1
fi
cmp "${chart_package}" "${digest_archives[0]}"
echo "chart_digest=${chart_digest}" >> "${GITHUB_OUTPUT}"
