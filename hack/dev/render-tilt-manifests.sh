#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
KUSTOMIZE_BIN="${KUSTOMIZE_BIN:-${ROOT_DIR}/bin/kustomize}"

if [ ! -x "${KUSTOMIZE_BIN}" ]; then
  if command -v kustomize >/dev/null 2>&1; then
    KUSTOMIZE_BIN="$(command -v kustomize)"
  else
    echo "error: kustomize is required. Run 'make bootstrap' first." >&2
    exit 1
  fi
fi

OPERATOR_VERSION="${OPERATOR_VERSION:-edge}"
OPERATOR_INIT_IMAGE_REPOSITORY="${OPERATOR_INIT_IMAGE_REPOSITORY:-ghcr.io/dc-tec/openbao-init}"
OPERATOR_BACKUP_IMAGE_REPOSITORY="${OPERATOR_BACKUP_IMAGE_REPOSITORY:-ghcr.io/dc-tec/openbao-backup}"
OPERATOR_UPGRADE_IMAGE_REPOSITORY="${OPERATOR_UPGRADE_IMAGE_REPOSITORY:-ghcr.io/dc-tec/openbao-upgrade}"
TILT_MANAGER_IMAGE="${TILT_MANAGER_IMAGE:-controller}"

tmpdir="$(mktemp -d)"
trap 'rm -rf "${tmpdir}"' EXIT

cp -R "${ROOT_DIR}/config" "${tmpdir}/config"

python3 - "$tmpdir/config/manager/controller.yaml" "$tmpdir/config/manager/provisioner.yaml" \
  "${OPERATOR_VERSION}" \
  "${OPERATOR_INIT_IMAGE_REPOSITORY}" \
  "${OPERATOR_BACKUP_IMAGE_REPOSITORY}" \
  "${OPERATOR_UPGRADE_IMAGE_REPOSITORY}" <<'PY'
import pathlib
import re
import sys

paths = [pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2])]
operator_version = sys.argv[3]
extra_env = [
    ("OPERATOR_INIT_IMAGE_REPOSITORY", sys.argv[4]),
    ("OPERATOR_BACKUP_IMAGE_REPOSITORY", sys.argv[5]),
    ("OPERATOR_UPGRADE_IMAGE_REPOSITORY", sys.argv[6]),
]


def replace_env_value(text: str, name: str, value: str) -> str:
    pattern = re.compile(
        rf'(^\s*-\s*name:\s*{re.escape(name)}\s*$\n^\s*value:\s*)(?:"[^"]*"|[^\n#]+)',
        re.MULTILINE,
    )
    if pattern.search(text):
        return pattern.sub(lambda m: f'{m.group(1)}"{value}"', text, count=1)
    return text


def insert_env_after(text: str, anchor: str, name: str, value: str) -> str:
    anchor_pattern = re.compile(
        rf'(^(\s*)-\s*name:\s*{re.escape(anchor)}\s*$\n^\s*value:\s*(?:"[^"]*"|[^\n#]+)\s*$)',
        re.MULTILINE,
    )

    def repl(match: re.Match[str]) -> str:
        indent = match.group(2)
        block = f'\n{indent}- name: {name}\n{indent}  value: "{value}"'
        return match.group(1) + block

    updated, count = anchor_pattern.subn(repl, text, count=1)
    if count == 0:
        raise SystemExit(f"could not find anchor env var {anchor!r} to insert {name!r}")
    return updated


for path in paths:
    text = path.read_text(encoding="utf-8")
    text = replace_env_value(text, "OPERATOR_VERSION", operator_version)
    for name, value in extra_env:
        updated = replace_env_value(text, name, value)
        if updated == text:
            text = insert_env_after(text, "OPERATOR_VERSION", name, value)
        else:
            text = updated
    path.write_text(text, encoding="utf-8")
PY

(
  cd "${tmpdir}/config/manager"
  "${KUSTOMIZE_BIN}" edit set image "controller=${TILT_MANAGER_IMAGE}"
)

"${KUSTOMIZE_BIN}" build "${tmpdir}/config/default" 2> >(grep -Fv "'patchesJson6902' is deprecated" >&2)
