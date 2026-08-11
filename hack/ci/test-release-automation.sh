#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
VALIDATOR="${ROOT_DIR}/hack/ci/validate-release-as-request.sh"
CHART_PREPARER="${ROOT_DIR}/hack/ci/prepare-release-chart.sh"
SPDX_NORMALIZER="${ROOT_DIR}/hack/ci/normalize-spdx-json.sh"
POST_RELEASE_VERIFIER="${ROOT_DIR}/hack/ci/verify-post-release.sh"
RELEASE_WORKFLOW="${ROOT_DIR}/.github/workflows/release.yml"
RELEASE_PR_GATE_WORKFLOW="${ROOT_DIR}/.github/workflows/release-pr-gate.yml"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

fail() {
  echo "release automation test failed: $*" >&2
  exit 1
}

expect_valid_request() {
  local target_branch="$1"
  local version="$2"
  local expected_branch="$3"
  local actual_branch

  actual_branch="$(
    env -u GITHUB_OUTPUT \
      TARGET_BRANCH="${target_branch}" \
      VERSION="${version}" \
      bash "${VALIDATOR}"
  )"
  [[ "${actual_branch}" == "${expected_branch}" ]] || {
    fail "expected marker '${expected_branch}', got '${actual_branch}'"
  }
}

expect_invalid_request() {
  local target_branch="$1"
  local version="$2"

  if TARGET_BRANCH="${target_branch}" VERSION="${version}" bash "${VALIDATOR}" >/dev/null 2>&1; then
    fail "expected target='${target_branch}' version='${version}' to be rejected"
  fi
}

write_chart_fixture() {
  local chart_file="$1"

  cat > "${chart_file}" <<'EOF'
apiVersion: v2
name: openbao-operator
version: 0.5.0
annotations:
  artifacthub.io/prerelease: "false"
  artifacthub.io/containsSecurityUpdates: 'false'
  artifacthub.io/changes: |
    - kind: changed
      description: "stale"
  artifacthub.io/images: |
    - name: openbao-operator
      image: ghcr.io/example/openbao-operator:stale
EOF
}

assert_contains() {
  local file="$1"
  local expected="$2"

  grep -Fq -- "${expected}" "${file}" || fail "expected '${expected}' in ${file}"
}

assert_not_contains() {
  local file="$1"
  local unexpected="$2"

  if grep -Fq -- "${unexpected}" "${file}"; then
    fail "did not expect '${unexpected}' in ${file}"
  fi
}

bash -n "${VALIDATOR}" "${CHART_PREPARER}" "${SPDX_NORMALIZER}" "${POST_RELEASE_VERIFIER}"

assert_contains "${RELEASE_WORKFLOW}" "Setup Helm 3 compatibility client"
assert_contains "${RELEASE_WORKFLOW}" 'HELM: ${{ steps.helm4.outputs.helm-path }}'
assert_contains "${RELEASE_WORKFLOW}" 'HELM_INSTALL: ${{ steps.helm3.outputs.helm-path }}'

assert_contains "${RELEASE_PR_GATE_WORKFLOW}" "pull_request_review:"
assert_contains "${RELEASE_PR_GATE_WORKFLOW}" "      - dismissed"
assert_contains "${RELEASE_PR_GATE_WORKFLOW}" 'head_sha="$(gh api'
assert_contains "${RELEASE_PR_GATE_WORKFLOW}" 'review_state="$(jq -r'
assert_contains "${RELEASE_PR_GATE_WORKFLOW}" 'review_commit="$(jq -r'
assert_not_contains "${RELEASE_PR_GATE_WORKFLOW}" "    paths:"

assert_contains "${POST_RELEASE_VERIFIER}" 'if [[ "${VERSION}" == *-* ]]; then'
assert_contains "${POST_RELEASE_VERIFIER}" 'if [[ "${is_prerelease}" != "${expected_prerelease}" ]]; then'
assert_contains "${POST_RELEASE_VERIFIER}" "github_release_prerelease_flag_verified: true"

spdx_fixture="${tmp_dir}/normalizer.spdx.json"
cat > "${spdx_fixture}" <<'EOF'
{
  "SPDXID": "SPDXRef-DOCUMENT",
  "spdxVersion": "SPDX-2.3",
  "creationInfo": {
    "created": "2040-01-01T00:00:00Z",
    "creators": ["Tool: release-automation-test"]
  },
  "dataLicense": "CC0-1.0",
  "documentNamespace": "https://example.test/original",
  "files": [
    {
      "SPDXID": "SPDXRef-File-A",
      "checksums": [{"algorithm": "SHA256", "checksumValue": "aa"}],
      "fileName": "a"
    }
  ],
  "name": "normalizer-test",
  "packages": [],
  "relationships": []
}
EOF
SOURCE_DATE_EPOCH=1700000000 bash "${SPDX_NORMALIZER}" "${spdx_fixture}"
python3 - "${spdx_fixture}" <<'PY'
import json
import sys
from pathlib import Path

document = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
checksum = document["files"][0]["checksums"][0]
if set(checksum) != {"algorithm", "checksumValue"}:
    raise SystemExit(f"normalizer added fields to checksum object: {sorted(checksum)}")
creation_info = document["creationInfo"]
unexpected = {
    "packages",
    "relationships",
    "files",
    "annotations",
    "hasExtractedLicensingInfos",
}.intersection(creation_info)
if unexpected:
    raise SystemExit(f"normalizer added fields to creationInfo: {sorted(unexpected)}")
PY

expect_valid_request "main" "0.5.0-rc.1" "automation/release-as-main-0.5.0-rc.1"
expect_valid_request "release-0.5" "0.5.1" "automation/release-as-release-0.5-0.5.1"
expect_invalid_request "release-as-main-0.5.0" "0.5.0"
expect_invalid_request "release-0.5.1" "0.5.1"
expect_invalid_request "release-0.5" "0.6.0"
expect_invalid_request "main" "0.5"
expect_invalid_request "main" "00.5.0"
expect_invalid_request "main" "0.5.0-rc.01"

marker_branch="$(env -u GITHUB_OUTPUT TARGET_BRANCH=main VERSION=0.5.0-rc.1 bash "${VALIDATOR}")"
if [[ "${marker_branch}" == release-* ]]; then
  fail "marker branch '${marker_branch}' overlaps the release branch namespace"
fi

for workflow in release-please.yml release-tag.yml; do
  if grep -Fq -- "- 'release-*'" "${ROOT_DIR}/.github/workflows/${workflow}"; then
    fail "${workflow} uses the broad release-* branch trigger"
  fi
done

output_file="${tmp_dir}/github-output"
GITHUB_OUTPUT="${output_file}" TARGET_BRANCH=main VERSION=0.5.0-rc.1 bash "${VALIDATOR}"
assert_contains "${output_file}" "marker_branch=automation/release-as-main-0.5.0-rc.1"

changelog_file="${tmp_dir}/CHANGELOG.md"
cat > "${changelog_file}" <<'EOF'
# Changelog

## [0.5.0](https://example.test/0.5.0) (2026-08-04)

### Bug Fixes

* **release:** finalize stable metadata
* **backup:** add immutable backup evidence

## [0.5.0-rc.2](https://example.test/0.5.0-rc.2) (2026-08-03)

### Features

* **backup:** add immutable backup evidence
* **controller:** add rc2-only reconciliation guard

## [0.5.0-rc.1](https://example.test/0.5.0-rc.1) (2026-08-02)

### Bug Fixes

* **deps:** resolve security vulnerabilities in example dependencies

### Features

* **backup:** add immutable backup evidence

## [0.4.0](https://example.test/0.4.0) (2026-07-01)

### Features

* **legacy:** older release entry
EOF

rc_chart_dir="${tmp_dir}/rc-chart"
mkdir -p "${rc_chart_dir}"
write_chart_fixture "${rc_chart_dir}/Chart.yaml"
CHART_DIR="${rc_chart_dir}" \
  CHANGELOG_FILE="${changelog_file}" \
  CHART_VERSION="0.5.0-rc.1" \
  OWNER="dc-tec" \
  bash "${CHART_PREPARER}"

assert_contains "${rc_chart_dir}/Chart.yaml" 'artifacthub.io/prerelease: "true"'
assert_contains "${rc_chart_dir}/Chart.yaml" "artifacthub.io/containsSecurityUpdates: 'true'"
assert_contains "${rc_chart_dir}/Chart.yaml" "kind: security"
assert_contains "${rc_chart_dir}/Chart.yaml" "deps: resolve security vulnerabilities in example dependencies"
assert_contains "${rc_chart_dir}/Chart.yaml" "backup: add immutable backup evidence"
assert_not_contains "${rc_chart_dir}/Chart.yaml" "release: finalize stable metadata"
assert_not_contains "${rc_chart_dir}/Chart.yaml" "controller: add rc2-only reconciliation guard"

stable_chart_dir="${tmp_dir}/stable-chart"
mkdir -p "${stable_chart_dir}"
write_chart_fixture "${stable_chart_dir}/Chart.yaml"
CHART_DIR="${stable_chart_dir}" \
  CHANGELOG_FILE="${changelog_file}" \
  CHART_VERSION="0.5.0" \
  OWNER="dc-tec" \
  bash "${CHART_PREPARER}"

stable_chart="${stable_chart_dir}/Chart.yaml"
assert_contains "${stable_chart}" 'artifacthub.io/prerelease: "false"'
assert_contains "${stable_chart}" "artifacthub.io/containsSecurityUpdates: 'true'"
assert_contains "${stable_chart}" "release: finalize stable metadata"
assert_contains "${stable_chart}" "backup: add immutable backup evidence"
assert_contains "${stable_chart}" "controller: add rc2-only reconciliation guard"
assert_contains "${stable_chart}" "deps: resolve security vulnerabilities in example dependencies"
assert_not_contains "${stable_chart}" "legacy: older release entry"

backup_change_count="$(grep -Fc "backup: add immutable backup evidence" "${stable_chart}")"
[[ "${backup_change_count}" == "1" ]] || fail "expected rolled-up changes to be deduplicated"

echo "release automation tests passed"
