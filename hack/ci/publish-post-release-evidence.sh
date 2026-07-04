#!/usr/bin/env bash

set -euo pipefail

fail() {
  echo "error: $*" >&2
  exit 1
}

warn() {
  echo "::warning::$*"
}

require_cmd() {
  local cmd="$1"
  command -v "${cmd}" >/dev/null 2>&1 || fail "required command not found: ${cmd}"
}

upsert_release_pr_comment() {
  local pr_number="$1"
  local pr_url="$2"
  local evidence_sha="$3"
  local body_file="$4"
  local marker
  local comments_json
  local comment_id
  local payload_file

  marker="<!-- openbao-operator:post-release-verification:${VERSION} -->"
  cat > "${body_file}" <<EOF
${marker}
## Post-release verification passed

- Release: \`${VERSION}\`
- GitHub Release: ${RELEASE_URL}
- Release workflow run: ${RELEASE_RUN_URL}
- Verification workflow run: ${VERIFICATION_RUN_URL}
- Chart digest: \`${CHART_DIGEST}\`
- Evidence artifact: ${VERIFICATION_RUN_URL} (artifact: \`${EVIDENCE_ARTIFACT_NAME}\`)
- Evidence sha256: \`${evidence_sha}\`

The published release assets, checksum signature, Helm chart signature, provenance index, and release-please cleanup checks have been verified.
EOF

  comments_json="$(gh api "repos/${REPO}/issues/${pr_number}/comments?per_page=100" --paginate --slurp)"
  comment_id="$(
    jq -r --arg marker "${marker}" \
      '[.[][] | select((.body // "") | contains($marker))][0].id // empty' \
      <<<"${comments_json}"
  )"

  payload_file="${TMPDIR}/release-pr-comment.json"
  jq -n --rawfile body "${body_file}" '{body: $body}' > "${payload_file}"

  if [[ -n "${comment_id}" ]]; then
    gh api -X PATCH "repos/${REPO}/issues/comments/${comment_id}" --input "${payload_file}" >/dev/null ||
      fail "failed to update post-release verification comment on ${pr_url}"
    echo "Updated post-release verification comment on ${pr_url}"
  else
    gh api -X POST "repos/${REPO}/issues/${pr_number}/comments" --input "${payload_file}" >/dev/null ||
      fail "failed to create post-release verification comment on ${pr_url}"
    echo "Created post-release verification comment on ${pr_url}"
  fi
}

resolve_release_pr() {
  local pr_candidates
  local release_pr_json
  local match_count

  pr_candidates="$(
    gh pr list \
      --repo "${REPO}" \
      --state merged \
      --search "${VERSION} in:title" \
      --limit 50 \
      --json number,title,url,mergeCommit
  )"

  release_pr_json="$(
    jq \
      --arg version "${VERSION}" \
      --arg tag_head_sha "${TAG_HEAD_SHA}" \
      '[.[] | select((.title | startswith("chore(")) and (.title | endswith("): release " + $version)))]
       | if ($tag_head_sha | length) > 0 then
           (map(select((.mergeCommit.oid // "") == $tag_head_sha)) as $exact
             | if ($exact | length) == 1 then $exact else . end)
         else
           .
         end' \
      <<<"${pr_candidates}"
  )"
  match_count="$(jq 'length' <<<"${release_pr_json}")"

  if [[ "${match_count}" != "1" ]]; then
    warn "expected exactly one merged release PR for ${VERSION}, found ${match_count}; skipping PR comment"
    jq -r '.[] | "- #\(.number) \(.title) (\(.url))"' <<<"${pr_candidates}" >&2 || true
    return 1
  fi

  RELEASE_PR_NUMBER="$(jq -r '.[0].number' <<<"${release_pr_json}")"
  RELEASE_PR_URL="$(jq -r '.[0].url' <<<"${release_pr_json}")"
}

for cmd in gh jq sha256sum; do
  require_cmd "${cmd}"
done

: "${VERSION:?VERSION is required}"
: "${REPO:?REPO is required}"
: "${EVIDENCE_PATH:?EVIDENCE_PATH is required}"

TAG_HEAD_SHA="${TAG_HEAD_SHA:-}"
RELEASE_RUN_ID="${RELEASE_RUN_ID:-}"
VERIFICATION_RUN_URL="${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY:-${REPO}}/actions/runs/${GITHUB_RUN_ID:-}"
RELEASE_RUN_URL="${GITHUB_SERVER_URL:-https://github.com}/${REPO}/actions/runs/${RELEASE_RUN_ID}"
EVIDENCE_ARTIFACT_NAME="${EVIDENCE_ARTIFACT_NAME:-post-release-verification-${VERSION}}"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "${TMPDIR}"' EXIT

if [[ ! -s "${EVIDENCE_PATH}" ]]; then
  fail "evidence file does not exist or is empty: ${EVIDENCE_PATH}"
fi

release_json="$(gh release view "${VERSION}" --repo "${REPO}" --json url)"
RELEASE_URL="$(jq -r '.url' <<<"${release_json}")"
evidence_sha="$(sha256sum "${EVIDENCE_PATH}" | awk '{print $1}')"

CHART_DIGEST="$(jq -r '.chart.digest' "${EVIDENCE_PATH}")"
if [[ -z "${CHART_DIGEST}" || "${CHART_DIGEST}" == "null" ]]; then
  fail "chart digest missing from evidence file"
fi

RELEASE_PR_NUMBER=""
RELEASE_PR_URL=""
if resolve_release_pr; then
  upsert_release_pr_comment "${RELEASE_PR_NUMBER}" "${RELEASE_PR_URL}" "${evidence_sha}" "${TMPDIR}/release-pr-comment.md"
fi

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  {
    echo "evidence_artifact_name=${EVIDENCE_ARTIFACT_NAME}"
    echo "evidence_sha256=${evidence_sha}"
    echo "release_pr_url=${RELEASE_PR_URL}"
  } >> "${GITHUB_OUTPUT}"
fi
