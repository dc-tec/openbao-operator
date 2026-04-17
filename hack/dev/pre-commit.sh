#!/usr/bin/env bash
set -euo pipefail

if [[ "${OPENBAO_OPERATOR_SKIP_PRE_COMMIT:-}" == "1" ]]; then
	exit 0
fi

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

mapfile -t staged_files < <(git diff --cached --name-only --diff-filter=ACMR)

if [[ ${#staged_files[@]} -eq 0 ]]; then
	exit 0
fi

echo "[pre-commit] Checking staged diff for whitespace errors"
git diff --cached --check

go_files=()
for path in "${staged_files[@]}"; do
	if [[ "$path" == vendor/* ]]; then
		continue
	fi
	if [[ "$path" == *.go ]]; then
		go_files+=("$path")
	fi
done

if [[ ${#go_files[@]} -eq 0 ]]; then
	exit 0
fi

if [[ ! -x "./bin/golangci-lint" ]]; then
	echo "[pre-commit] Missing ./bin/golangci-lint. Run 'make bootstrap' first." >&2
	exit 1
fi

echo "[pre-commit] Formatting staged Go files"
gofmt -w "${go_files[@]}"
git add "${go_files[@]}"

pkg_patterns=()
while IFS= read -r pkg_pattern; do
	pkg_patterns+=("$pkg_pattern")
done < <(
	for file in "${go_files[@]}"; do
		dir="$(dirname "$file")"
		printf './%s\n' "$dir"
	done | sort -u
)

echo "[pre-commit] Running golangci-lint on affected Go paths"
"./bin/golangci-lint" run "${pkg_patterns[@]}"
