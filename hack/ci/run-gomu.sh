#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

timestamp_utc() {
  date -u +"%Y%m%dT%H%M%SZ"
}

usage() {
  cat <<'EOF'
Usage: run-gomu.sh [options]

Options:
  --path <value>           Mutation target path(s), comma-separated. Default: ./internal
  --workers <n>            Number of gomu workers. Default: 4
  --timeout <seconds>      Per-mutation test timeout in seconds. Default: 30
  --incremental <bool>     gomu incremental mode (true/false). Default: false
  --go-flags <value>       Value for GOFLAGS while running gomu/go test. Default: empty
  --go-mem-limit <value>   Value for GOMEMLIMIT while running gomu/go test. Default: empty
  --output-dir <path>      Artifact output directory. Default: dist/mutation/<timestamp>
  --ci-mode <bool>         gomu CI mode (true/false). Default: false
  --fail-on-gate <bool>    Fail on threshold gate (true/false). Default: false
  --base-branch <name>     Base branch used by gomu incremental logic. Default: main
  --top-survivors <n>      Max survived mutants in summary. Default: 20
  --verbose <bool>         Verbose mode for this script and gomu (true/false). Default: false
  --help                   Show this help.

Environment overrides:
  GOMU_BIN, GOMU_PATHS, GOMU_PATH, GOMU_WORKERS, GOMU_TIMEOUT, GOMU_INCREMENTAL,
  GOMU_GOFLAGS, GOMU_GOMEMLIMIT, GOMU_OUTPUT_DIR, GOMU_CI_MODE,
  GOMU_FAIL_ON_GATE, GOMU_BASE_BRANCH, GOMU_TOP_SURVIVORS, GOMU_VERBOSE
EOF
}

normalize_bool() {
  local value
  value="$(echo "${1:-}" | tr '[:upper:]' '[:lower:]')"
  case "${value}" in
    true|false) echo "${value}" ;;
    *) echo "invalid" ;;
  esac
}

sanitize_name() {
  local value="$1"
  local out
  out="$(echo "${value}" | tr '/. ' '___' | tr -cd '[:alnum:]_-' )"
  if [[ -z "${out}" ]]; then
    out="path"
  fi
  echo "${out}"
}

GOMU_BIN="${GOMU_BIN:-gomu}"
PATH_LIST="${GOMU_PATHS:-${GOMU_PATH:-./internal}}"
WORKERS="${GOMU_WORKERS:-4}"
TIMEOUT="${GOMU_TIMEOUT:-30}"
INCREMENTAL="$(normalize_bool "${GOMU_INCREMENTAL:-false}")"
GO_FLAGS="${GOMU_GOFLAGS:-}"
GO_MEM_LIMIT="${GOMU_GOMEMLIMIT:-}"
OUTPUT_DIR="${GOMU_OUTPUT_DIR:-${REPO_ROOT}/dist/mutation/$(timestamp_utc)}"
CI_MODE="$(normalize_bool "${GOMU_CI_MODE:-false}")"
FAIL_ON_GATE="$(normalize_bool "${GOMU_FAIL_ON_GATE:-false}")"
BASE_BRANCH="${GOMU_BASE_BRANCH:-main}"
TOP_SURVIVORS="${GOMU_TOP_SURVIVORS:-20}"
VERBOSE="$(normalize_bool "${GOMU_VERBOSE:-false}")"

if [[ "${INCREMENTAL}" == "invalid" || "${CI_MODE}" == "invalid" || "${FAIL_ON_GATE}" == "invalid" || "${VERBOSE}" == "invalid" ]]; then
  echo "Invalid boolean environment value (expected true/false)." >&2
  exit 1
fi

while [[ $# -gt 0 ]]; do
  case "$1" in
    --path)
      PATH_LIST="${2:-}"
      shift 2
      ;;
    --workers)
      WORKERS="${2:-}"
      shift 2
      ;;
    --timeout)
      TIMEOUT="${2:-}"
      shift 2
      ;;
    --incremental)
      INCREMENTAL="$(normalize_bool "${2:-}")"
      shift 2
      ;;
    --go-flags)
      GO_FLAGS="${2:-}"
      shift 2
      ;;
    --go-mem-limit)
      GO_MEM_LIMIT="${2:-}"
      shift 2
      ;;
    --output-dir)
      OUTPUT_DIR="${2:-}"
      shift 2
      ;;
    --ci-mode)
      CI_MODE="$(normalize_bool "${2:-}")"
      shift 2
      ;;
    --fail-on-gate)
      FAIL_ON_GATE="$(normalize_bool "${2:-}")"
      shift 2
      ;;
    --base-branch)
      BASE_BRANCH="${2:-}"
      shift 2
      ;;
    --top-survivors)
      TOP_SURVIVORS="${2:-}"
      shift 2
      ;;
    --verbose)
      VERBOSE="$(normalize_bool "${2:-}")"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ "${INCREMENTAL}" == "invalid" || "${CI_MODE}" == "invalid" || "${FAIL_ON_GATE}" == "invalid" || "${VERBOSE}" == "invalid" ]]; then
  echo "Invalid boolean flag value (expected true/false)." >&2
  exit 1
fi

if ! [[ "${WORKERS}" =~ ^[0-9]+$ ]] || [[ "${WORKERS}" -lt 1 ]]; then
  echo "workers must be a positive integer, got: ${WORKERS}" >&2
  exit 1
fi
if ! [[ "${TIMEOUT}" =~ ^[0-9]+$ ]] || [[ "${TIMEOUT}" -lt 1 ]]; then
  echo "timeout must be a positive integer, got: ${TIMEOUT}" >&2
  exit 1
fi
if ! [[ "${TOP_SURVIVORS}" =~ ^[0-9]+$ ]]; then
  echo "top-survivors must be a non-negative integer, got: ${TOP_SURVIVORS}" >&2
  exit 1
fi

if [[ "${OUTPUT_DIR}" != /* ]]; then
  OUTPUT_DIR="${REPO_ROOT}/${OUTPUT_DIR}"
fi

mkdir -p "${OUTPUT_DIR}/reports" "${OUTPUT_DIR}/logs"

tmp_root="$(mktemp -d)"
cleanup() {
  rm -rf "${tmp_root}"
}
trap cleanup EXIT

IFS=',' read -r -a raw_paths <<< "${PATH_LIST}"
paths=()
for p in "${raw_paths[@]}"; do
  trimmed="$(echo "${p}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  if [[ -n "${trimmed}" ]]; then
    paths+=("${trimmed}")
  fi
done

if [[ "${#paths[@]}" -eq 0 ]]; then
  echo "No valid --path entries were provided." >&2
  exit 1
fi

report_files=()
overall_status=0

for idx in "${!paths[@]}"; do
  path_value="${paths[$idx]}"
  run_name="$(sanitize_name "${path_value}")"
  run_dir="${tmp_root}/run-${idx}"
  run_log="${OUTPUT_DIR}/logs/${run_name}.log"
  run_report="${OUTPUT_DIR}/reports/${run_name}.json"
  mkdir -p "${run_dir}"

  target_path="${path_value}"
  if [[ "${target_path}" != /* ]]; then
    target_path="${REPO_ROOT}/${target_path}"
  fi

  if [[ ! -e "${target_path}" ]]; then
    cat > "${run_log}" <<EOF
run-gomu: target path does not exist: ${target_path}
EOF
    cat > "${run_report}" <<EOF
{
  "totalFiles": 0,
  "processedFiles": 0,
  "totalMutants": 0,
  "killedMutants": 0,
  "results": [],
  "statistics": {
    "killed": 0,
    "survived": 0,
    "timedOut": 0,
    "errors": 1,
    "notViable": 0,
    "mutationScore": 0
  }
}
EOF
    report_files+=("${run_report}")
    overall_status=1
    continue
  fi

  cmd=("${GOMU_BIN}")
  if [[ "${VERBOSE}" == "true" ]]; then
    cmd+=("-v")
  fi
  cmd+=("run" "${target_path}")
  cmd+=("--workers=${WORKERS}")
  cmd+=("--timeout=${TIMEOUT}")
  cmd+=("--incremental=${INCREMENTAL}")
  cmd+=("--output=json")
  cmd+=("--fail-on-gate=${FAIL_ON_GATE}")
  cmd+=("--base-branch=${BASE_BRANCH}")
  if [[ "${CI_MODE}" == "true" ]]; then
    cmd+=("--ci-mode")
  fi

  if [[ "${VERBOSE}" == "true" ]]; then
    echo "Executing: ${cmd[*]}" | tee "${run_log}"
    if [[ -n "${GO_FLAGS}" ]]; then
      echo "Using GOFLAGS=${GO_FLAGS}" | tee -a "${run_log}"
    fi
    if [[ -n "${GO_MEM_LIMIT}" ]]; then
      echo "Using GOMEMLIMIT=${GO_MEM_LIMIT}" | tee -a "${run_log}"
    fi
  fi

  run_env=()
  if [[ -n "${GO_FLAGS}" ]]; then
    run_env+=("GOFLAGS=${GO_FLAGS}")
  fi
  if [[ -n "${GO_MEM_LIMIT}" ]]; then
    run_env+=("GOMEMLIMIT=${GO_MEM_LIMIT}")
  fi

  set +e
  (
    cd "${run_dir}"
    if [[ "${#run_env[@]}" -gt 0 ]]; then
      env "${run_env[@]}" "${cmd[@]}"
    else
      "${cmd[@]}"
    fi
  ) >> "${run_log}" 2>&1
  run_exit=$?
  set -e

  if [[ "${run_exit}" -ne 0 ]]; then
    overall_status=1
  fi

  if [[ -f "${run_dir}/mutation-report.json" ]]; then
    cp "${run_dir}/mutation-report.json" "${run_report}"
  else
    cat > "${run_report}" <<EOF
{
  "totalFiles": 0,
  "processedFiles": 0,
  "totalMutants": 0,
  "killedMutants": 0,
  "results": [],
  "statistics": {
    "killed": 0,
    "survived": 0,
    "timedOut": 0,
    "errors": 1,
    "notViable": 0,
    "mutationScore": 0
  }
}
EOF
  fi

  report_files+=("${run_report}")
done

summary_txt="${OUTPUT_DIR}/mutation-summary.txt"
summary_md="${OUTPUT_DIR}/mutation-summary.md"
merged_report="${OUTPUT_DIR}/mutation-report.json"

summary_args=()
for report in "${report_files[@]}"; do
  summary_args+=("--report" "${report}")
done

set +e
(
  cd "${REPO_ROOT}"
  go run ./hack/tools/mutation_summary/main.go \
    "${summary_args[@]}" \
    --top "${TOP_SURVIVORS}" \
    --format text \
    --output "${summary_txt}" \
    --write-json "${merged_report}"
)
summary_exit=$?
set -e
if [[ "${summary_exit}" -ne 0 ]]; then
  overall_status=1
  cat > "${summary_txt}" <<EOF
Mutation summary generation failed.
See logs under: ${OUTPUT_DIR}/logs
EOF
  if [[ ! -f "${merged_report}" ]]; then
    cat > "${merged_report}" <<EOF
{
  "totalFiles": 0,
  "processedFiles": 0,
  "totalMutants": 0,
  "killedMutants": 0,
  "results": [],
  "statistics": {
    "killed": 0,
    "survived": 0,
    "timedOut": 0,
    "errors": 1,
    "notViable": 0,
    "mutationScore": 0
  }
}
EOF
  fi
fi

set +e
(
  cd "${REPO_ROOT}"
  go run ./hack/tools/mutation_summary/main.go \
    "${summary_args[@]}" \
    --top "${TOP_SURVIVORS}" \
    --format markdown \
    --output "${summary_md}"
)
markdown_exit=$?
set -e
if [[ "${markdown_exit}" -ne 0 ]]; then
  overall_status=1
  cat > "${summary_md}" <<EOF
## Mutation Summary

Summary generation failed.
See logs under: ${OUTPUT_DIR}/logs
EOF
fi

cat > "${OUTPUT_DIR}/run-config.txt" <<EOF
path=${PATH_LIST}
workers=${WORKERS}
timeout=${TIMEOUT}
incremental=${INCREMENTAL}
ci_mode=${CI_MODE}
fail_on_gate=${FAIL_ON_GATE}
base_branch=${BASE_BRANCH}
top_survivors=${TOP_SURVIVORS}
go_flags=${GO_FLAGS}
go_mem_limit=${GO_MEM_LIMIT}
gomu_bin=${GOMU_BIN}
EOF

echo "Mutation artifacts:"
echo "  merged report: ${merged_report}"
echo "  text summary: ${summary_txt}"
echo "  markdown summary: ${summary_md}"
echo "  per-path reports: ${OUTPUT_DIR}/reports"
echo "  logs: ${OUTPUT_DIR}/logs"

exit "${overall_status}"
