#!/usr/bin/env bash

set -euo pipefail

fail() {
  echo "error: $*" >&2
  exit 1
}

require_cmd() {
  local cmd="$1"
  command -v "${cmd}" >/dev/null 2>&1 || fail "required command not found: ${cmd}"
}

for cmd in gh jq; do
  require_cmd "${cmd}"
done

: "${VERSION:?VERSION is required}"
: "${REPO:?REPO is required}"

TAG_HEAD_SHA="${TAG_HEAD_SHA:-}"

pr_candidates="$(
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
  echo "expected exactly one merged release PR for ${VERSION}, found ${match_count}" >&2
  jq -r '.[] | "- #\(.number) \(.title) (\(.url))"' <<<"${pr_candidates}" >&2 || true
  exit 1
fi

release_pr_number="$(jq -r '.[0].number' <<<"${release_pr_json}")"
release_pr_url="$(jq -r '.[0].url' <<<"${release_pr_json}")"
labels="$(jq -r '.[0].labels[]?.name' <<<"${release_pr_json}")"
removed=0
pending_labels=(
  "autorelease: pending"
  "autorelease:pending"
)

for label in "${pending_labels[@]}"; do
  if grep -Fxq "${label}" <<<"${labels}"; then
    encoded_label="$(jq -rn --arg label "${label}" '$label | @uri')"
    gh api -X DELETE "repos/${REPO}/issues/${release_pr_number}/labels/${encoded_label}" >/dev/null
    echo "Removed '${label}' from release PR ${release_pr_url}"
    removed=1
  fi
done

if [[ "${removed}" == "0" ]]; then
  echo "No release-please pending label found on ${release_pr_url}"
fi
