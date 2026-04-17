#!/usr/bin/env bash
set -euo pipefail

if [[ "${OPENBAO_OPERATOR_SKIP_PRE_PUSH:-}" == "1" ]]; then
	exit 0
fi

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

echo "[pre-push] Running make lint-ci"
make lint-ci
