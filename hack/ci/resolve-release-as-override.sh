#!/usr/bin/env bash

set -euo pipefail

: "${EVENT_NAME:?EVENT_NAME is required}"
: "${TARGET_BRANCH:?TARGET_BRANCH is required}"

DISPATCH_RELEASE_AS="${DISPATCH_RELEASE_AS:-}"
COMMIT_SUBJECT="${COMMIT_SUBJECT:-}"
COMMIT_IS_EMPTY="${COMMIT_IS_EMPTY:-false}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
VALIDATOR="${ROOT_DIR}/hack/ci/validate-release-as-request.sh"

case "${COMMIT_IS_EMPTY}" in
  true|false) ;;
  *)
    echo "COMMIT_IS_EMPTY must be true or false, got '${COMMIT_IS_EMPTY}'" >&2
    exit 1
    ;;
esac

release_as=""
case "${EVENT_NAME}" in
  workflow_dispatch)
    release_as="${DISPATCH_RELEASE_AS}"
    ;;
  push)
    marker_regex='^chore\((main|release-[0-9]+\.[0-9]+)\): request release ([^[:space:]]+) \(#[1-9][0-9]*\)$'
    if [[ "${COMMIT_SUBJECT}" =~ ${marker_regex} ]]; then
      if [[ "${COMMIT_IS_EMPTY}" != "true" ]]; then
        echo "Release-As marker squash commit must be empty" >&2
        exit 1
      fi

      marker_target="${BASH_REMATCH[1]}"
      release_as="${BASH_REMATCH[2]}"
      if [[ "${marker_target}" != "${TARGET_BRANCH}" ]]; then
        echo "marker target '${marker_target}' does not match '${TARGET_BRANCH}'" >&2
        exit 1
      fi
    fi
    ;;
  *)
    echo "unsupported event '${EVENT_NAME}'" >&2
    exit 1
    ;;
esac

if [[ -n "${release_as}" ]]; then
  env -u GITHUB_OUTPUT \
    TARGET_BRANCH="${TARGET_BRANCH}" \
    VERSION="${release_as}" \
    bash "${VALIDATOR}" >/dev/null
fi

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  echo "release_as=${release_as}" >> "${GITHUB_OUTPUT}"
else
  echo "${release_as}"
fi
