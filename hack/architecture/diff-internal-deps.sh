#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

DEFAULT_BASELINE_REPORT="${REPO_ROOT}/dist/architecture/internal-dependency-report.baseline.md"
DEFAULT_CURRENT_REPORT="${REPO_ROOT}/dist/architecture/internal-dependency-report.md"

usage() {
  cat <<USAGE
Usage: $0 [baseline-report] [current-report]

Compares two internal dependency reports and prints a report-only delta.

Defaults:
  baseline-report: ${DEFAULT_BASELINE_REPORT}
  current-report:  ${DEFAULT_CURRENT_REPORT}

Tip:
  Create/update a local baseline snapshot with:
    cp ${DEFAULT_CURRENT_REPORT} ${DEFAULT_BASELINE_REPORT}
USAGE
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ $# -gt 2 ]]; then
  usage
  exit 1
fi

BASELINE_REPORT="${1:-${DEFAULT_BASELINE_REPORT}}"
CURRENT_REPORT="${2:-${DEFAULT_CURRENT_REPORT}}"

if [[ ! -f "${BASELINE_REPORT}" ]]; then
  echo "Error: baseline report not found: ${BASELINE_REPORT}" >&2
  usage >&2
  exit 1
fi

if [[ ! -f "${CURRENT_REPORT}" ]]; then
  echo "Error: current report not found: ${CURRENT_REPORT}" >&2
  usage >&2
  exit 1
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

extract_summary_value() {
  local report_file="$1"
  local key="$2"
  awk -v key="${key}" '
    index($0, "- " key ": ") == 1 {
      sub("- " key ": ", "")
      print
      exit
    }
  ' "${report_file}"
}

extract_top_table() {
  local report_file="$1"
  local heading="$2"
  awk -F'|' -v heading="## ${heading}" '
    $0 == heading {
      in_section = 1
      next
    }
    in_section && /^## / {
      exit
    }
    in_section && $0 ~ /^\| [0-9]+ \|/ {
      imports = $2
      pkg = $3
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", imports)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", pkg)
      gsub(/`/, "", pkg)
      if (pkg != "") {
        printf "%s\t%s\n", pkg, imports
      }
    }
  ' "${report_file}"
}

extract_policy_warnings() {
  local report_file="$1"
  awk '
    $0 == "## Policy Warnings" {
      in_section = 1
      next
    }
    in_section && /^## / {
      exit
    }
    in_section && /^- / {
      warning = $0
      sub(/^- /, "", warning)
      if (warning != "None" && warning != "None.") {
        print warning
      }
    }
  ' "${report_file}"
}

print_numeric_delta() {
  local label="$1"
  local before="$2"
  local after="$3"

  if [[ "${before}" =~ ^[0-9]+$ && "${after}" =~ ^[0-9]+$ ]]; then
    local delta=$((after - before))
    printf -- "- %s: %s -> %s (delta %+d)\n" "${label}" "${before}" "${after}" "${delta}"
    return
  fi

  printf -- "- %s: %s -> %s\n" "${label}" "${before:-n/a}" "${after:-n/a}"
}

print_top_list_delta() {
  local title="$1"
  local baseline_file="$2"
  local current_file="$3"
  local merged_file="${TMP_DIR}/${title// /-}-merged.tsv"

  awk -F'\t' '
    FNR == NR {
      baseline[$1] = $2
      pkgs[$1] = 1
      next
    }
    {
      current[$1] = $2
      pkgs[$1] = 1
    }
    END {
      for (pkg in pkgs) {
        before = (pkg in baseline) ? baseline[pkg] : 0
        after = (pkg in current) ? current[pkg] : 0
        if (before != after) {
          delta = after - before
          abs_delta = delta
          if (abs_delta < 0) {
            abs_delta = -abs_delta
          }
          printf "%d\t%s\t%d\t%d\t%d\n", abs_delta, pkg, before, after, delta
        }
      }
    }
  ' "${baseline_file}" "${current_file}" | sort -nr -k1,1 -k2,2 > "${merged_file}"

  echo "${title} delta:"
  if [[ ! -s "${merged_file}" ]]; then
    echo "- No changes in top list entries."
    return
  fi

  local line_count
  line_count="$(wc -l < "${merged_file}" | tr -d ' ')"

  while IFS=$'\t' read -r _abs_delta pkg before after delta; do
    if [[ "${before}" -eq 0 ]]; then
      printf -- '- `%s`: added to current top list (%s)\n' "${pkg}" "${after}"
    elif [[ "${after}" -eq 0 ]]; then
      printf -- '- `%s`: removed from current top list (was %s)\n' "${pkg}" "${before}"
    else
      printf -- '- `%s`: %s -> %s (delta %+d)\n' "${pkg}" "${before}" "${after}" "${delta}"
    fi
  done < <(head -n 15 "${merged_file}")

  if (( line_count > 15 )); then
    printf -- "- ... plus %d more changed entries\n" "$((line_count - 15))"
  fi
}

baseline_nodes="$(extract_summary_value "${BASELINE_REPORT}" "Nodes")"
baseline_edges="$(extract_summary_value "${BASELINE_REPORT}" "Edges")"
baseline_cycle="$(extract_summary_value "${BASELINE_REPORT}" "Cycle check")"

current_nodes="$(extract_summary_value "${CURRENT_REPORT}" "Nodes")"
current_edges="$(extract_summary_value "${CURRENT_REPORT}" "Edges")"
current_cycle="$(extract_summary_value "${CURRENT_REPORT}" "Cycle check")"

baseline_fanin_file="${TMP_DIR}/baseline-fanin.tsv"
current_fanin_file="${TMP_DIR}/current-fanin.tsv"
baseline_fanout_file="${TMP_DIR}/baseline-fanout.tsv"
current_fanout_file="${TMP_DIR}/current-fanout.tsv"

extract_top_table "${BASELINE_REPORT}" "Top Fan-In" > "${baseline_fanin_file}"
extract_top_table "${CURRENT_REPORT}" "Top Fan-In" > "${current_fanin_file}"
extract_top_table "${BASELINE_REPORT}" "Top Fan-Out" > "${baseline_fanout_file}"
extract_top_table "${CURRENT_REPORT}" "Top Fan-Out" > "${current_fanout_file}"

baseline_warnings_file="${TMP_DIR}/baseline-warnings.txt"
current_warnings_file="${TMP_DIR}/current-warnings.txt"

extract_policy_warnings "${BASELINE_REPORT}" | sort -u > "${baseline_warnings_file}"
extract_policy_warnings "${CURRENT_REPORT}" | sort -u > "${current_warnings_file}"

new_warnings_file="${TMP_DIR}/new-warnings.txt"
resolved_warnings_file="${TMP_DIR}/resolved-warnings.txt"

comm -13 "${baseline_warnings_file}" "${current_warnings_file}" > "${new_warnings_file}" || true
comm -23 "${baseline_warnings_file}" "${current_warnings_file}" > "${resolved_warnings_file}" || true

echo "Internal dependency report delta"
echo "- Baseline: ${BASELINE_REPORT}"
echo "- Current: ${CURRENT_REPORT}"
echo

echo "Summary delta:"
print_numeric_delta "Nodes" "${baseline_nodes}" "${current_nodes}"
print_numeric_delta "Edges" "${baseline_edges}" "${current_edges}"
printf -- "- Cycle check: %s -> %s\n" "${baseline_cycle:-n/a}" "${current_cycle:-n/a}"
echo

print_top_list_delta "Top Fan-In" "${baseline_fanin_file}" "${current_fanin_file}"
echo
print_top_list_delta "Top Fan-Out" "${baseline_fanout_file}" "${current_fanout_file}"
echo

echo "Policy warning delta:"
echo "- New warnings since baseline:"
if [[ -s "${new_warnings_file}" ]]; then
  sed 's/^/  - /' "${new_warnings_file}"
else
  echo "  - None"
fi

echo "- Resolved warnings since baseline:"
if [[ -s "${resolved_warnings_file}" ]]; then
  sed 's/^/  - /' "${resolved_warnings_file}"
else
  echo "  - None"
fi

echo
if [[ -s "${current_warnings_file}" ]]; then
  echo "Current warning set:"
  sed 's/^/- /' "${current_warnings_file}"
else
  echo "Current warning set:"
  echo "- None"
fi

exit 0
