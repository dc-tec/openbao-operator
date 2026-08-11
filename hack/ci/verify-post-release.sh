#!/usr/bin/env bash

set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
usage: VERSION=X.Y.Z [REPO=dc-tec/openbao-operator] hack/ci/verify-post-release.sh

Verifies the post-release invariants that should hold after the Release workflow
has published a stable or prerelease release.

Environment:
  VERSION       Required release version, for example 0.3.0.
  REPO          GitHub repository. Default: dc-tec/openbao-operator.
  GIT_REMOTE    Git remote used for branch/tag checks. Default: https://github.com/${REPO}.git.
  ALLOW_DRAFT   Set to 1 to allow a draft GitHub Release. Default: 0.
  EVIDENCE_OUT  Optional path where a JSON verification evidence file is written.
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
EVIDENCE_OUT="${EVIDENCE_OUT:-}"
VERIFIED_AT="${VERIFIED_AT:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
RELEASE_RUN_ID="${RELEASE_RUN_ID:-}"
VERIFICATION_RUN_ID="${GITHUB_RUN_ID:-}"
VERIFICATION_RUN_URL="${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY:-${REPO}}/actions/runs/${GITHUB_RUN_ID:-}"

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
tag_refs="$(git ls-remote --tags "${GIT_REMOTE}" "refs/tags/${VERSION}" "refs/tags/${VERSION}^{}")" ||
  fail "release tag ${VERSION} was not found on ${GIT_REMOTE}"
tag_head_sha="$(awk '$2 ~ /\^\{\}$/ {print $1; exit}' <<<"${tag_refs}")"
if [[ -z "${tag_head_sha}" ]]; then
  tag_head_sha="$(awk -v ref="refs/tags/${VERSION}" '$2 == ref {print $1; exit}' <<<"${tag_refs}")"
fi
if [[ -z "${tag_head_sha}" ]]; then
  fail "could not resolve release tag ${VERSION} target commit from ${GIT_REMOTE}"
fi

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

expected_prerelease="false"
if [[ "${VERSION}" == *-* ]]; then
  expected_prerelease="true"
fi
is_prerelease="$(jq -r '.isPrerelease' <<<"${release_json}")"
if [[ "${is_prerelease}" != "${expected_prerelease}" ]]; then
  fail "GitHub Release ${VERSION} prerelease flag is '${is_prerelease}', expected '${expected_prerelease}'"
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

info "checking release-please pending label cleanup"
release_pr_candidates="$(
  gh pr list \
    --repo "${REPO}" \
    --state merged \
    --search "${VERSION} in:title" \
    --limit 50 \
    --json number,title,url,mergeCommit,labels
)"
release_pr_json="$(
  jq \
    --arg version "${VERSION}" \
    --arg tag_head_sha "${tag_head_sha}" \
    '[.[] | select((.title | startswith("chore(")) and (.title | endswith("): release " + $version)))]
     | if ($tag_head_sha | length) > 0 then
         (map(select((.mergeCommit.oid // "") == $tag_head_sha)) as $exact
           | if ($exact | length) == 1 then $exact else . end)
       else
         .
       end' \
    <<<"${release_pr_candidates}"
)"
release_pr_match_count="$(jq 'length' <<<"${release_pr_json}")"
if [[ "${release_pr_match_count}" != "1" ]]; then
  echo "expected exactly one merged release PR for ${VERSION}, found ${release_pr_match_count}" >&2
  jq -r '.[] | "- #\(.number) \(.title) (\(.url))"' <<<"${release_pr_candidates}" >&2 || true
  exit 1
fi

release_pr_number="$(jq -r '.[0].number' <<<"${release_pr_json}")"
release_pr_url="$(jq -r '.[0].url' <<<"${release_pr_json}")"
pending_release_labels="$(
  jq -r '.[]?.labels[]?.name | select(. == "autorelease: pending" or . == "autorelease:pending")' \
    <<<"${release_pr_json}"
)"
if [[ -n "${pending_release_labels}" ]]; then
  echo "${pending_release_labels}" >&2
  fail "release PR ${release_pr_url} still has release-please pending label"
fi

if [[ -n "${EVIDENCE_OUT}" ]]; then
  info "writing post-release verification evidence: ${EVIDENCE_OUT}"
  mkdir -p "$(dirname "${EVIDENCE_OUT}")"
  assets_file="${tmpdir}/release-assets.txt"
  printf '%s\n' "${asset_names[@]}" > "${assets_file}"
  jq -n \
    --arg schema_version "1" \
    --arg repo "${REPO}" \
    --arg version "${VERSION}" \
    --arg verified_at "${VERIFIED_AT}" \
    --arg release_run_id "${RELEASE_RUN_ID}" \
    --arg verification_run_id "${VERIFICATION_RUN_ID}" \
    --arg verification_run_url "${VERIFICATION_RUN_URL}" \
    --arg release_url "${release_url}" \
    --arg release_pr_number "${release_pr_number}" \
    --arg release_pr_url "${release_pr_url}" \
    --arg identity "${identity}" \
    --arg chart_ref "${chart_ref}" \
    --arg chart_digest "${chart_digest}" \
    --slurpfile release <(printf '%s\n' "${release_json}") \
    --slurpfile provenance "${tmpdir}/provenance-index.json" \
    --rawfile assets "${assets_file}" \
    'def optional_string($value):
      if ($value | length) > 0 then $value else null end;

    {
      schema_version: $schema_version,
      repository: $repo,
      version: $version,
      verified_at: $verified_at,
      release_run: {
        id: optional_string($release_run_id)
      },
      verification_run: {
        id: optional_string($verification_run_id),
        url: (if ($verification_run_id | length) > 0 then $verification_run_url else null end)
      },
      release: {
        url: $release_url,
        tag: $release[0].tagName,
        draft: $release[0].isDraft,
        prerelease: $release[0].isPrerelease,
        assets: ($assets | split("\n") | map(select(length > 0)))
      },
      release_pr: {
        number: ($release_pr_number | tonumber),
        url: $release_pr_url
      },
      identity_constraints: {
        certificate_identity: $identity,
        certificate_oidc_issuer: "https://token.actions.githubusercontent.com"
      },
      chart: {
        ref: $chart_ref,
        digest: $chart_digest
      },
      published_provenance: $provenance[0],
      checks: {
        remote_tag_present: true,
        github_release_published: true,
        github_release_prerelease_flag_verified: true,
        required_assets_present: true,
        checksums_signature_verified: true,
        helm_chart_published: true,
        helm_chart_signature_verified: true,
        no_open_release_please_prs: true,
        no_stale_release_please_branches: true,
        release_please_pending_label_cleared: true
      }
    }' > "${EVIDENCE_OUT}"
fi

info "post-release verification passed for ${VERSION}"
