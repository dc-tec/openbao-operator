#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
usage: VERSION=X.Y.Z [REPO=dc-tec/openbao-operator] hack/ci/verify-post-release.sh

Verifies the post-release invariants that should hold after the Release workflow
has published a stable release.

Environment:
  VERSION       Required release version, for example 0.3.0.
  REPO          GitHub repository. Default: dc-tec/openbao-operator.
  GIT_REMOTE    Git remote used for branch/tag checks. Default: https://github.com/${REPO}.git.
  ALLOW_DRAFT   Set to 1 to allow a draft GitHub Release. Default: 0.
USAGE
}

fail() {
  echo "error: $*" >&2
  exit 1
}

info() {
  echo "==> $*"
}

require_cmd() {
  local cmd="$1"
  command -v "${cmd}" >/dev/null 2>&1 || fail "required command not found: ${cmd}"
}

VERSION="${VERSION:-${1:-}}"
REPO="${REPO:-dc-tec/openbao-operator}"
OWNER="${REPO%%/*}"
GIT_REMOTE="${GIT_REMOTE:-https://github.com/${REPO}.git}"
ALLOW_DRAFT="${ALLOW_DRAFT:-0}"

if [[ "${VERSION}" == "-h" || "${VERSION}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ -z "${VERSION}" ]]; then
  usage
  exit 2
fi

if ! [[ "${VERSION}" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-.+][0-9A-Za-z.-]+)?$ ]]; then
  fail "VERSION must be SemVer, got '${VERSION}'"
fi

for cmd in gh jq git docker cosign; do
  require_cmd "${cmd}"
done

required_assets=(
  install.yaml
  crds.yaml
  checksums.txt
  checksums.txt.bundle
  checksums.txt.sigstore.json
  checksums.intoto.jsonl
  sbom-openbao-operator.spdx.json
  sbom-openbao-init.spdx.json
  sbom-openbao-backup.spdx.json
  sbom-openbao-upgrade.spdx.json
  provenance-index.json
)

info "checking remote tag ${VERSION}"
git ls-remote --exit-code --tags "${GIT_REMOTE}" "refs/tags/${VERSION}" >/dev/null ||
  fail "release tag ${VERSION} was not found on ${GIT_REMOTE}"

info "checking GitHub Release ${VERSION}"
release_json="$(
  gh release view "${VERSION}" \
    --repo "${REPO}" \
    --json assets,isDraft,isPrerelease,tagName,url
)"

release_tag="$(jq -r '.tagName' <<<"${release_json}")"
if [[ "${release_tag}" != "${VERSION}" ]]; then
  fail "GitHub Release tagName is '${release_tag}', expected '${VERSION}'"
fi

is_draft="$(jq -r '.isDraft' <<<"${release_json}")"
if [[ "${is_draft}" == "true" && "${ALLOW_DRAFT}" != "1" ]]; then
  fail "GitHub Release ${VERSION} is still a draft"
fi

mapfile -t asset_names < <(jq -r '.assets[].name' <<<"${release_json}" | LC_ALL=C sort)
missing_assets=()
for asset in "${required_assets[@]}"; do
  if ! printf '%s\n' "${asset_names[@]}" | grep -Fxq "${asset}"; then
    missing_assets+=("${asset}")
  fi
done
if (( ${#missing_assets[@]} > 0 )); then
  printf 'missing release assets:\n' >&2
  printf '  - %s\n' "${missing_assets[@]}" >&2
  exit 1
fi

release_url="$(jq -r '.url' <<<"${release_json}")"
info "release assets present: ${release_url}"

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

info "downloading signature evidence"
gh release download "${VERSION}" \
  --repo "${REPO}" \
  --dir "${tmpdir}" \
  --clobber \
  --pattern checksums.txt \
  --pattern checksums.txt.bundle \
  --pattern provenance-index.json

identity="https://github.com/${REPO}/.github/workflows/release.yml@refs/tags/${VERSION}"

info "verifying checksums signature"
cosign verify-blob \
  --new-bundle-format=true \
  --bundle "${tmpdir}/checksums.txt.bundle" \
  --certificate-identity "${identity}" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  "${tmpdir}/checksums.txt" >/dev/null

provenance_tag="$(jq -r '.release.tag' "${tmpdir}/provenance-index.json")"
if [[ "${provenance_tag}" != "${VERSION}" ]]; then
  fail "provenance-index.json release tag is '${provenance_tag}', expected '${VERSION}'"
fi

chart_ref="ghcr.io/${OWNER}/charts/openbao-operator:${VERSION}"
info "checking Helm chart publication: ${chart_ref}"
chart_digest="$(
  docker buildx imagetools inspect "${chart_ref}" --format '{{json .Manifest.Digest}}' | tr -d '"'
)"
if [[ -z "${chart_digest}" || "${chart_digest}" == "null" ]]; then
  fail "could not resolve chart digest for ${chart_ref}"
fi

info "verifying Helm chart signature"
cosign verify \
  --new-bundle-format=true \
  --certificate-identity "${identity}" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  "ghcr.io/${OWNER}/charts/openbao-operator@${chart_digest}" >/dev/null

info "checking for open release-please PRs"
open_release_prs="$(
  gh pr list \
    --repo "${REPO}" \
    --state open \
    --json number,title,headRefName,url \
    --jq '.[] | select(.headRefName | startswith("release-please--branches--")) | "#\(.number) \(.title) [\(.headRefName)] \(.url)"'
)"
if [[ -n "${open_release_prs}" ]]; then
  echo "${open_release_prs}" >&2
  fail "unexpected open release-please PRs remain"
fi

info "checking for stale release-please branches"
stale_branches="$(git ls-remote --heads "${GIT_REMOTE}" 'release-please--branches--*')"
if [[ -n "${stale_branches}" ]]; then
  echo "${stale_branches}" >&2
  fail "stale release-please branches remain"
fi

info "post-release verification passed for ${VERSION}"
