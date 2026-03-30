#!/usr/bin/env bash

set -euo pipefail

: "${REPO:?REPO is required (for example dc-tec/openbao-operator)}"

BASE_BRANCH="${BASE_BRANCH:-main}"
MANIFEST_FILE="${MANIFEST_FILE:-.release-please-manifest.json}"
CHART_FILE="${CHART_FILE:-charts/openbao-operator/Chart.yaml}"
DRY_RUN="${DRY_RUN:-0}"

GH_READ_TOKEN="${GH_READ_TOKEN:-${GH_TOKEN:-}}"
GH_WRITE_TOKEN="${GH_WRITE_TOKEN:-${GH_TOKEN:-}}"

if [[ -z "${GH_READ_TOKEN}" ]]; then
  echo "GH_READ_TOKEN (or GH_TOKEN) is required" >&2
  exit 1
fi

if [[ "${DRY_RUN}" != "1" && -z "${GH_WRITE_TOKEN}" ]]; then
  echo "GH_WRITE_TOKEN (or GH_TOKEN) is required unless DRY_RUN=1" >&2
  exit 1
fi

require_file() {
  local path="$1"
  if [[ ! -f "${path}" ]]; then
    echo "required file not found: ${path}" >&2
    exit 1
  fi
}

chart_value() {
  local file="$1"
  local key="$2"
  sed -nE "s/^${key}:[[:space:]]*\"?([^\"#[:space:]]+)\"?[[:space:]]*(#.*)?$/\\1/p" "${file}" | head -n1
}

sanitize_release_notes() {
  awk '
    BEGIN {
      trim_leading = 1
      saw_robot = 0
    }
    NR == 1 && /^:robot: I have created a release/ {
      saw_robot = 1
      next
    }
    saw_robot && trim_leading && /^---[[:space:]]*$/ { next }
    saw_robot && trim_leading && /^[[:space:]]*$/ { next }
    {
      trim_leading = 0
      lines[++count] = $0
    }
    END {
      end = count
      if (count >= 1 && lines[count] ~ /^This PR was generated with \[Release Please\]/) {
        end = count - 1
        while (end > 0 && lines[end] ~ /^[[:space:]]*$/) {
          end--
        }
        if (end > 0 && lines[end] ~ /^---[[:space:]]*$/) {
          end--
        }
        while (end > 0 && lines[end] ~ /^[[:space:]]*$/) {
          end--
        }
      }

      for (i = 1; i <= end; i++) {
        print lines[i]
      }
    }
  '
}

gh_read() {
  GH_TOKEN="${GH_READ_TOKEN}" gh "$@"
}

gh_write() {
  GH_TOKEN="${GH_WRITE_TOKEN}" gh "$@"
}

require_file "${MANIFEST_FILE}"
require_file "${CHART_FILE}"

version="$(jq -er '."."' "${MANIFEST_FILE}")"
chart_version="$(chart_value "${CHART_FILE}" "version")"
chart_app_version="$(chart_value "${CHART_FILE}" "appVersion")"

if [[ -z "${chart_version}" || -z "${chart_app_version}" ]]; then
  echo "failed to parse version/appVersion from ${CHART_FILE}" >&2
  exit 1
fi

if [[ "${version}" != "${chart_version}" || "${version}" != "${chart_app_version}" ]]; then
  echo "release version mismatch between ${MANIFEST_FILE} and ${CHART_FILE}" >&2
  echo "  manifest:   ${version}" >&2
  echo "  chart:      ${chart_version}" >&2
  echo "  appVersion: ${chart_app_version}" >&2
  exit 1
fi

release_pr_title="chore(${BASE_BRANCH}): release ${version}"
pr_candidates="$(
  gh_read pr list \
    --repo "${REPO}" \
    --state merged \
    --search "${version} in:title" \
    --limit 50 \
    --json number,title,url
)"

match_count="$(
  jq -r --arg title "${release_pr_title}" '[.[] | select(.title == $title)] | length' <<<"${pr_candidates}"
)"

if [[ "${match_count}" != "1" ]]; then
  echo "expected exactly one merged release PR titled '${release_pr_title}', found ${match_count}" >&2
  jq -r '.[] | "- #\(.number) \(.title) (\(.url))"' <<<"${pr_candidates}" >&2 || true
  exit 1
fi

release_pr_number="$(
  jq -r --arg title "${release_pr_title}" '[.[] | select(.title == $title)][0].number' <<<"${pr_candidates}"
)"

release_pr_json="$(
  gh_read pr view "${release_pr_number}" \
    --repo "${REPO}" \
    --json number,title,state,body,mergeCommit,mergedAt,url
)"

release_pr_state="$(jq -r '.state' <<<"${release_pr_json}")"
merge_oid="$(jq -r '.mergeCommit.oid // empty' <<<"${release_pr_json}")"

if [[ "${release_pr_state}" != "MERGED" || -z "${merge_oid}" ]]; then
  echo "release PR #${release_pr_number} is not a merged PR with a merge commit" >&2
  exit 1
fi

manifest_at_merge="$(git show "${merge_oid}:${MANIFEST_FILE}" | jq -er '."."')"
chart_version_at_merge="$(git show "${merge_oid}:${CHART_FILE}" | chart_value /dev/stdin "version")"
chart_app_version_at_merge="$(git show "${merge_oid}:${CHART_FILE}" | chart_value /dev/stdin "appVersion")"

if [[ "${manifest_at_merge}" != "${version}" || "${chart_version_at_merge}" != "${version}" || "${chart_app_version_at_merge}" != "${version}" ]]; then
  echo "release files at merge commit ${merge_oid} do not match ${version}" >&2
  echo "  manifest@merge:   ${manifest_at_merge}" >&2
  echo "  chart@merge:      ${chart_version_at_merge}" >&2
  echo "  appVersion@merge: ${chart_app_version_at_merge}" >&2
  exit 1
fi

notes_file="$(mktemp)"
trap 'rm -f "${notes_file}"' EXIT

jq -r '.body // empty' <<<"${release_pr_json}" | sanitize_release_notes > "${notes_file}"

if [[ ! -s "${notes_file}" ]]; then
  echo "release PR #${release_pr_number} body is empty after sanitization" >&2
  exit 1
fi

if git rev-parse -q --verify "refs/tags/${version}" >/dev/null 2>&1; then
  local_tag_commit="$(git rev-list -n1 "${version}")"
  if [[ "${local_tag_commit}" != "${merge_oid}" ]]; then
    echo "local tag ${version} points at ${local_tag_commit}, expected ${merge_oid}" >&2
    exit 1
  fi
elif git ls-remote --exit-code --tags origin "refs/tags/${version}" >/dev/null 2>&1; then
  git fetch --no-tags origin "refs/tags/${version}:refs/tags/${version}" >/dev/null 2>&1
  remote_tag_commit="$(git rev-list -n1 "${version}")"
  if [[ "${remote_tag_commit}" != "${merge_oid}" ]]; then
    echo "remote tag ${version} points at ${remote_tag_commit}, expected ${merge_oid}" >&2
    exit 1
  fi
else
  if [[ "${DRY_RUN}" == "1" ]]; then
    echo "[dry-run] would create annotated tag ${version} at ${merge_oid}"
  else
    git config user.name "github-actions[bot]"
    git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
    git tag -a "${version}" "${merge_oid}" -m "${version}"
    git push origin "refs/tags/${version}"
  fi
fi

is_prerelease="false"
if [[ "${version}" == *-* ]]; then
  is_prerelease="true"
fi

release_json=""
if release_json="$(gh_write release view "${version}" --repo "${REPO}" --json tagName,isDraft,isPrerelease,name,body,url 2>/dev/null)"; then
  release_exists="true"
else
  release_exists="false"
fi

if [[ "${release_exists}" == "false" ]]; then
  if [[ "${DRY_RUN}" == "1" ]]; then
    echo "[dry-run] would create draft release ${version} from merged PR #${release_pr_number}"
    exit 0
  fi

  release_args=(
    release create "${version}"
    --repo "${REPO}"
    --verify-tag
    --draft
    --title "${version}"
    --notes-file "${notes_file}"
  )
  if [[ "${is_prerelease}" == "true" ]]; then
    release_args+=(--prerelease)
  fi
  gh_write "${release_args[@]}"
  exit 0
fi

release_name="$(jq -r '.name // ""' <<<"${release_json}")"
release_is_draft="$(jq -r '.isDraft' <<<"${release_json}")"
release_is_prerelease="$(jq -r '.isPrerelease' <<<"${release_json}")"

if [[ "${release_is_draft}" != "true" ]]; then
  if [[ "${release_name}" != "${version}" ]]; then
    echo "release ${version} exists with unexpected title '${release_name}'" >&2
    exit 1
  fi
  if [[ "${release_is_prerelease}" != "${is_prerelease}" ]]; then
    echo "release ${version} prerelease flag is ${release_is_prerelease}, expected ${is_prerelease}" >&2
    exit 1
  fi
  echo "release ${version} already exists and is published; leaving it unchanged" >&2
  exit 0
fi

if [[ "${is_prerelease}" == "false" && "${release_is_prerelease}" == "true" ]]; then
  echo "draft release ${version} is marked prerelease but the resolved version is stable" >&2
  exit 1
fi

if [[ "${DRY_RUN}" == "1" ]]; then
  echo "[dry-run] would refresh draft release ${version} from merged PR #${release_pr_number}"
  exit 0
fi

edit_args=(
  release edit "${version}"
  --repo "${REPO}"
  --verify-tag
  --draft
  --title "${version}"
  --notes-file "${notes_file}"
)

if [[ "${is_prerelease}" == "true" ]]; then
  edit_args+=(--prerelease)
fi

gh_write "${edit_args[@]}"
