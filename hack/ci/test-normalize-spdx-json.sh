#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NORMALIZER="${ROOT_DIR}/hack/ci/normalize-spdx-json.sh"

: "${SPDX_SCHEMA_2_2:?SPDX_SCHEMA_2_2 is required; run through devenv}"
: "${SPDX_SCHEMA_2_3:?SPDX_SCHEMA_2_3 is required; run through devenv}"
: "${SPDX_FIXTURE_2_2:?SPDX_FIXTURE_2_2 is required; run through devenv}"
: "${SPDX_FIXTURE_2_3:?SPDX_FIXTURE_2_3 is required; run through devenv}"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

fail() {
  echo "SPDX normalizer test failed: $*" >&2
  exit 1
}

validate_fixture() {
  local version="$1"
  local source="$2"
  local schema="$3"
  local target="${tmp_dir}/spdx-${version}.json"
  local first="${tmp_dir}/spdx-${version}.first.json"
  local invalid="${tmp_dir}/spdx-${version}.invalid.json"

  cp "${source}" "${target}"
  chmod u+w "${target}"
  check-jsonschema --schemafile "${schema}" "${target}" >/dev/null

  SOURCE_DATE_EPOCH=1700000000 bash "${NORMALIZER}" "${target}"
  check-jsonschema --schemafile "${schema}" "${target}" >/dev/null

  cp "${target}" "${first}"
  SOURCE_DATE_EPOCH=1700000000 bash "${NORMALIZER}" "${target}"
  cmp -s "${first}" "${target}" || fail "SPDX ${version} normalization is not idempotent"

  cp "${target}" "${invalid}"
  chmod u+w "${invalid}"
  python3 - "${invalid}" <<'PY'
import json
import sys
from pathlib import Path

path = Path(sys.argv[1])
document = json.loads(path.read_text(encoding="utf-8"))
document["creationInfo"]["packages"] = None
path.write_text(json.dumps(document), encoding="utf-8")
PY
  if check-jsonschema --schemafile "${schema}" "${invalid}" >/dev/null 2>&1; then
    fail "SPDX ${version} schema check accepted an injected creationInfo field"
  fi
}

bash -n "${NORMALIZER}"
validate_fixture "2.2" "${SPDX_FIXTURE_2_2}" "${SPDX_SCHEMA_2_2}"
validate_fixture "2.3" "${SPDX_FIXTURE_2_3}" "${SPDX_SCHEMA_2_3}"

echo "SPDX normalizer schema tests passed"
