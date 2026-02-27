#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

OUT_DIR="${OUT_DIR:-${REPO_ROOT}/dist/architecture}"
EDGE_FILE="${OUT_DIR}/internal-dependency-edges.tsv"
DOT_FILE="${OUT_DIR}/internal-dependency-graph.dot"
MMD_FILE="${OUT_DIR}/internal-dependency-graph.mmd"
REPORT_FILE="${OUT_DIR}/internal-dependency-report.md"

mkdir -p "${OUT_DIR}"

cd "${REPO_ROOT}"
MODULE_PATH="$(go list -m)"
root_controller_pkg_present="false"
if go list ./... | grep -qx "${MODULE_PATH}/internal/controller"; then
  root_controller_pkg_present="true"
fi

# Runtime scope only: api/v1alpha1, cmd/*, internal/*
go list -f '{{.ImportPath}}|{{join .Imports ","}}' ./... \
  | awk -F'|' -v mod="${MODULE_PATH}" '
      function is_runtime_pkg(p) {
        return p == mod "/api/v1alpha1" || index(p, mod "/cmd") == 1 || index(p, mod "/internal") == 1
      }
      {
        src = $1
        if (!is_runtime_pkg(src)) {
          next
        }
        dep_count = split($2, deps, ",")
        for (i = 1; i <= dep_count; i++) {
          dep = deps[i]
          if (is_runtime_pkg(dep)) {
            print src "\t" dep
          }
        }
      }
    ' \
  | sort -u > "${EDGE_FILE}"

edge_count="$(wc -l < "${EDGE_FILE}" | tr -d ' ')"
node_count="$(awk -F'\t' '{print $1; print $2}' "${EDGE_FILE}" | sort -u | wc -l | tr -d ' ')"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

TOP_IN_FILE="${TMP_DIR}/top-in.txt"
TOP_OUT_FILE="${TMP_DIR}/top-out.txt"
WARNINGS_FILE="${TMP_DIR}/warnings.txt"
ACTIVE_EXCEPTIONS_FILE="${TMP_DIR}/active-policy-exceptions.tsv"
POLICY_EXCEPTIONS_FILE="${SCRIPT_DIR}/dependency-policy-exceptions.tsv"

awk -F'\t' -v mod="${MODULE_PATH}" '
  function rel(p) {
    sub(mod "/", "", p)
    return p
  }
  {
    dep = rel($2)
    indeg[dep]++
  }
  END {
    for (pkg in indeg) {
      printf "%d\t%s\n", indeg[pkg], pkg
    }
  }
' "${EDGE_FILE}" | sort -nr | head -n 15 > "${TOP_IN_FILE}"

awk -F'\t' -v mod="${MODULE_PATH}" '
  function rel(p) {
    sub(mod "/", "", p)
    return p
  }
  {
    src = rel($1)
    outdeg[src]++
  }
  END {
    for (pkg in outdeg) {
      printf "%d\t%s\n", outdeg[pkg], pkg
    }
  }
' "${EDGE_FILE}" | sort -nr | head -n 15 > "${TOP_OUT_FILE}"

# Policy warnings (report-only)
awk -F'\t' -v mod="${MODULE_PATH}" -v policy_exceptions="${POLICY_EXCEPTIONS_FILE}" -v active_exceptions="${ACTIVE_EXCEPTIONS_FILE}" '
  function rel(p) {
    sub(mod "/", "", p)
    return p
  }
  BEGIN {
    while ((getline line < policy_exceptions) > 0) {
      if (line ~ /^[[:space:]]*#/ || line ~ /^[[:space:]]*$/) {
        continue
      }
      fields = split(line, parts, "\t")
      if (fields < 2) {
        continue
      }
      key = parts[1] SUBSEP parts[2]
      adapter_adapter_exception[key] = 1
      if (fields >= 3) {
        adapter_adapter_exception_reason[key] = parts[3]
      }
    }
    close(policy_exceptions)
  }
  function is_service_pkg(p) {
    return p ~ /^internal\/(backup|restore|upgrade|upgrade\/bluegreen|upgrade\/rolling|infra|certs|init|provisioner)$/
  }
  function is_controller_impl_pkg(p) {
    return p ~ /^internal\/controller\//
  }
  function is_adapter_pkg(p) {
    return p ~ /^internal\/(kube|openbao|storage|auth|cluster|config|raft|security|storageenv|operationlock|revision|probe)$/
  }
  {
    src = rel($1)
    dep = rel($2)

    if (dep == "internal/controller" && !is_controller_impl_pkg(src) && src != "internal/controller") {
      print "[shared-controller-import] " src " -> " dep
    }
    if (is_adapter_pkg(src) && is_controller_impl_pkg(dep)) {
      print "[adapter->controller] " src " -> " dep
    }
    if (is_adapter_pkg(src) && is_service_pkg(dep)) {
      print "[adapter->service] " src " -> " dep
    }
    if (is_adapter_pkg(src) && is_adapter_pkg(dep) && src != dep) {
      key = src SUBSEP dep
      if (adapter_adapter_exception[key]) {
        if (!seen_exception[key]) {
          seen_exception[key] = 1
          reason = adapter_adapter_exception_reason[key]
          printf "%s\t%s\t%s\n", src, dep, reason >> active_exceptions
        }
      } else {
        print "[adapter->adapter] " src " -> " dep
      }
    }
    if (dep == "internal/interfaces") {
      print "[deprecated-interfaces] " src " -> " dep
    }
  }
' "${EDGE_FILE}" | sort -u > "${WARNINGS_FILE}"

if [ "${root_controller_pkg_present}" = "true" ]; then
  printf '[shared-controller-package-present] internal/controller package exists; keep shared helpers in internal/predicates or internal/observability\n' >> "${WARNINGS_FILE}"
fi

constants_in="$(awk -F'\t' -v mod="${MODULE_PATH}" '$2 == mod "/internal/constants" {count++} END {print count + 0}' "${EDGE_FILE}")"
openbaocluster_out="$(awk -F'\t' -v mod="${MODULE_PATH}" '$1 == mod "/internal/controller/openbaocluster" {count++} END {print count + 0}' "${EDGE_FILE}")"

if [ "${constants_in}" -gt 12 ]; then
  printf '[threshold] internal/constants fan-in %s exceeds target <= 12\n' "${constants_in}" >> "${WARNINGS_FILE}"
fi

if [ "${openbaocluster_out}" -gt 12 ]; then
  printf '[threshold] internal/controller/openbaocluster fan-out %s exceeds target <= 12\n' "${openbaocluster_out}" >> "${WARNINGS_FILE}"
fi

# Graph cycle check (best effort)
cycle_status="acyclic"
if command -v tsort >/dev/null 2>&1; then
  TSORT_INPUT="${TMP_DIR}/tsort-input.txt"
  TSORT_ERR="${TMP_DIR}/tsort-err.txt"
  awk -F'\t' '{print $1"\n"$2}' "${EDGE_FILE}" > "${TSORT_INPUT}"
  if ! tsort "${TSORT_INPUT}" > /dev/null 2> "${TSORT_ERR}"; then
    cycle_status="cyclic"
    printf '[cycle] tsort reported dependency cycle(s)\n' >> "${WARNINGS_FILE}"
    sed -n '1,5p' "${TSORT_ERR}" | sed 's/^/[cycle-detail] /' >> "${WARNINGS_FILE}"
  fi
else
  cycle_status="unknown (tsort unavailable)"
fi

# Graphviz DOT output
{
  echo 'digraph OpenBaoOperatorInternalDependencies {'
  echo '  rankdir=LR;'
  echo '  graph [fontsize=10, fontname="Helvetica"];'
  echo '  node [shape=box, fontsize=9, fontname="Helvetica"];'
  echo '  edge [fontsize=8, fontname="Helvetica", arrowsize=0.6];'
  awk -F'\t' -v mod="${MODULE_PATH}" '
    function rel(p) {
      sub(mod "/", "", p)
      return p
    }
    {
      printf "  \"%s\" -> \"%s\";\n", rel($1), rel($2)
    }
  ' "${EDGE_FILE}"
  echo '}'
} > "${DOT_FILE}"

# Mermaid output
awk -F'\t' -v mod="${MODULE_PATH}" '
  function rel(p) {
    sub(mod "/", "", p)
    return p
  }
  function node_id(pkg, id) {
    id = pkg
    gsub(/[^A-Za-z0-9]/, "_", id)
    return "n_" id
  }
  function node_class(pkg) {
    if (pkg == "api/v1alpha1") {
      return "api"
    }
    if (index(pkg, "cmd") == 1) {
      return "cmd"
    }
    if (index(pkg, "internal/") == 1) {
      return "internal"
    }
    return "other"
  }
  {
    src = rel($1)
    dep = rel($2)

    src_id = node_id(src)
    dep_id = node_id(dep)

    if (!(src_id in labels)) {
      labels[src_id] = src
      classes[src_id] = node_class(src)
      order[++node_count] = src_id
    }

    if (!(dep_id in labels)) {
      labels[dep_id] = dep
      classes[dep_id] = node_class(dep)
      order[++node_count] = dep_id
    }

    edges[++edge_count] = src_id " --> " dep_id
  }
  END {
    print "graph LR"
    print "  classDef api fill:#eef2ff,stroke:#3730a3,color:#1e1b4b"
    print "  classDef cmd fill:#ecfeff,stroke:#0e7490,color:#083344"
    print "  classDef internal fill:#ecfdf3,stroke:#166534,color:#052e16"
    print "  classDef other fill:#f8fafc,stroke:#475569,color:#0f172a"
    print ""

    for (i = 1; i <= node_count; i++) {
      id = order[i]
      printf "  %s[\"%s\"]\n", id, labels[id]
    }

    print ""
    for (i = 1; i <= edge_count; i++) {
      printf "  %s\n", edges[i]
    }

    print ""
    for (i = 1; i <= node_count; i++) {
      id = order[i]
      printf "  class %s %s;\n", id, classes[id]
    }
  }
' "${EDGE_FILE}" > "${MMD_FILE}"

GENERATED_AT="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

{
  echo "# Internal Dependency Report"
  echo
  echo "Generated: ${GENERATED_AT}"
  echo "Mode: report-only (policy violations are warnings; exit code remains 0)"
  echo "Scope: runtime packages in api/v1alpha1, cmd/*, internal/*"
  echo
  echo "## Summary"
  echo
  echo "- Nodes: ${node_count}"
  echo "- Edges: ${edge_count}"
  echo "- Cycle check: ${cycle_status}"
  echo "- Shared controller package present: ${root_controller_pkg_present}"
  echo
  echo "## Artifacts"
  echo
  echo "- Edge list: ${EDGE_FILE}"
  echo "- Graphviz DOT: ${DOT_FILE}"
  echo "- Mermaid: ${MMD_FILE}"
  echo
  echo "## Top Fan-In"
  echo
  if [ -s "${TOP_IN_FILE}" ]; then
    echo "| Imports | Package |"
    echo "| ---: | :--- |"
    awk -F'\t' '{printf "| %s | `%s` |\n", $1, $2}' "${TOP_IN_FILE}"
  else
    echo "No edges found."
  fi
  echo
  echo "## Top Fan-Out"
  echo
  if [ -s "${TOP_OUT_FILE}" ]; then
    echo "| Imports | Package |"
    echo "| ---: | :--- |"
    awk -F'\t' '{printf "| %s | `%s` |\n", $1, $2}' "${TOP_OUT_FILE}"
  else
    echo "No edges found."
  fi
  echo
  echo "## Policy Warnings"
  echo
  if [ -s "${WARNINGS_FILE}" ]; then
    sed 's/^/- /' "${WARNINGS_FILE}"
  else
    echo "- None"
  fi
  echo
  echo "## Intentional Policy Exceptions"
  echo
  if [ -s "${ACTIVE_EXCEPTIONS_FILE}" ]; then
    echo "| Edge | Reason |"
    echo "| :--- | :--- |"
    awk -F'\t' '
      {
        reason = $3
        if (reason == "") {
          reason = "(unspecified)"
        }
        gsub(/\|/, "\\|", reason)
        printf "| `%s -> %s` | %s |\n", $1, $2, reason
      }
    ' "${ACTIVE_EXCEPTIONS_FILE}"
  else
    echo "- None"
  fi
} > "${REPORT_FILE}"

echo "Internal dependency report generated:"
echo "  ${REPORT_FILE}"
echo "  ${EDGE_FILE}"
echo "  ${DOT_FILE}"
echo "  ${MMD_FILE}"
echo ""
echo "Warnings are report-only; this command does not fail on policy findings."

exit 0
