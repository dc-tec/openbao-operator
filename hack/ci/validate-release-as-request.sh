#!/usr/bin/env bash

set -euo pipefail

: "${TARGET_BRANCH:?TARGET_BRANCH is required}"
: "${VERSION:?VERSION is required}"

if ! [[ "${TARGET_BRANCH}" =~ ^(main|release-[0-9]+\.[0-9]+)$ ]]; then
  echo "target_branch must be main or release-X.Y, got '${TARGET_BRANCH}'" >&2
  exit 1
fi

semver_core='(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)'
semver_prerelease='(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?'
semver_build='(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?'
semver_regex="^${semver_core}${semver_prerelease}${semver_build}$"
if ! [[ "${VERSION}" =~ ${semver_regex} ]]; then
  echo "version must be SemVer, got '${VERSION}'" >&2
  exit 1
fi

if [[ "${VERSION}" == *-* ]]; then
  prerelease="${VERSION#*-}"
  prerelease="${prerelease%%+*}"
  IFS='.' read -r -a prerelease_identifiers <<< "${prerelease}"
  for identifier in "${prerelease_identifiers[@]}"; do
    if [[ "${identifier}" =~ ^[0-9]+$ && "${identifier}" != "0" && "${identifier}" == 0* ]]; then
      echo "numeric prerelease identifiers must not contain leading zeroes, got '${VERSION}'" >&2
      exit 1
    fi
  done
fi

version_core="${VERSION%%[-+]*}"
IFS='.' read -r version_major version_minor _ <<< "${version_core}"
version_line="${version_major}.${version_minor}"
if [[ "${TARGET_BRANCH}" != "main" && "${TARGET_BRANCH}" != "release-${version_line}" ]]; then
  echo "version '${VERSION}' does not belong to release line '${TARGET_BRANCH}'" >&2
  exit 1
fi

marker_branch="automation/release-as-${TARGET_BRANCH}-${VERSION}"
if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  echo "marker_branch=${marker_branch}" >> "${GITHUB_OUTPUT}"
else
  echo "${marker_branch}"
fi
