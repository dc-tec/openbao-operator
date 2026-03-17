#!/usr/bin/env bash

set -euo pipefail

: "${FUZZTIME:=3s}"
: "${FUZZ_GOMAXPROCS:=4}"
: "${FUZZ_TARGET_FILTER:=}"
: "${FUZZ_ARTIFACT_DIR:=dist/fuzz}"

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${repo_root}"
mkdir -p "${FUZZ_ARTIFACT_DIR}"

sanitize_name() {
	local value="$1"
	value="${value#./}"
	value="${value//\//__}"
	value="${value//:/_}"
	value="${value// /_}"
	printf '%s' "${value}"
}

targets_manifest="${FUZZ_ARTIFACT_DIR}/targets.txt"
: > "${targets_manifest}"

mapfile -t entries < <(
	rg -n --no-heading '^func (Fuzz[[:alnum:]_]+)\(' cmd internal --glob '*fuzz_test.go' |
		sed -E 's#^([^:]+):[0-9]+:func (Fuzz[^(]+)\(.*#\1\t\2#' |
		LC_ALL=C sort -u
)

if [[ "${#entries[@]}" -eq 0 ]]; then
	echo "No fuzz targets found under cmd/ or internal/." >&2
	exit 1
fi

selected=0
for entry in "${entries[@]}"; do
	IFS=$'\t' read -r file fuzz_target <<< "${entry}"
	pkg="./$(dirname "${file}")"

	if [[ -n "${FUZZ_TARGET_FILTER}" ]]; then
		if [[ ! "${pkg}" =~ ${FUZZ_TARGET_FILTER} ]] && [[ ! "${fuzz_target}" =~ ${FUZZ_TARGET_FILTER} ]]; then
			continue
		fi
	fi

	selected=$((selected + 1))
	echo "${pkg} ${fuzz_target}" >> "${targets_manifest}"
	log_file="${FUZZ_ARTIFACT_DIR}/$(sanitize_name "${pkg}")__$(sanitize_name "${fuzz_target}").log"
	{
		echo "==> ${pkg} ${fuzz_target} (${FUZZTIME}, GOMAXPROCS=${FUZZ_GOMAXPROCS})"
		GOMAXPROCS="${FUZZ_GOMAXPROCS}" go test "${pkg}" -run='^$' -fuzz="^${fuzz_target}$" -fuzztime="${FUZZTIME}"
	} 2>&1 | tee "${log_file}"
done

if [[ "${selected}" -eq 0 ]]; then
	echo "No fuzz targets matched FUZZ_TARGET_FILTER=${FUZZ_TARGET_FILTER}." >&2
	exit 1
fi
