#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

if ! command -v docker >/dev/null 2>&1; then
  echo "error: docker is required" >&2
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  echo "error: go is required" >&2
  exit 1
fi

VERSIONS=("$@")
if [ ${#VERSIONS[@]} -eq 0 ]; then
  VERSIONS=("2.4.4" "2.5.5" "2.6.0")
fi

FILES=( "$ROOT_DIR"/internal/adapter/config/testdata/*.hcl )
if [ ${#FILES[@]} -eq 0 ]; then
  echo "error: no HCL fixtures found under internal/adapter/config/testdata" >&2
  exit 1
fi

REPORT_SCHEMA_DRIFT="${REPORT_SCHEMA_DRIFT:-false}"
TMPMODULE_GOFLAGS="${TMPMODULE_GOFLAGS:--mod=mod}"
REPO_GOFLAGS="${REPO_GOFLAGS:--mod=vendor}"
schema_tmp=""
if [ "${REPORT_SCHEMA_DRIFT}" = "true" ]; then
  schema_tmp="$(mktemp -d)"
  trap 'rm -rf "$schema_tmp"' EXIT
fi

for version in "${VERSIONS[@]}"; do
  echo "==> OpenBao config compatibility: $version"

  sha="$(docker run --rm "openbao/openbao:${version}" version | sed -n 's/.*(\([0-9a-f]\{40\}\)).*/\1/p')"
  if [ -z "${sha}" ]; then
    echo "error: failed to determine git SHA for openbao/openbao:${version}" >&2
    exit 1
  fi
  echo "    SHA: ${sha}"

  tmpdir="$(mktemp -d)"
  trap 'rm -rf "$tmpdir"' EXIT

  cat > "${tmpdir}/main.go" <<'GO'
package main

import (
	"fmt"
	"os"

	serverconfig "github.com/openbao/openbao/command/server"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: openbao-config-compat <file1.hcl> [file2.hcl...]")
		os.Exit(2)
	}

	var failed bool
	for _, path := range os.Args[1:] {
		b, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", path, err)
			failed = true
			continue
		}
		if _, err := serverconfig.ParseConfig(string(b), path); err != nil {
			fmt.Fprintf(os.Stderr, "parse %s: %v\n", path, err)
			failed = true
			continue
		}
	}

	if failed {
		os.Exit(1)
	}
}
GO

  (
    cd "${tmpdir}"
    go mod init tmp.example/openbao-config-compat >/dev/null 2>&1
    # OpenBao's root module is not a supported consumer module and uses a local
    # replace for sdk/v2. Module consumers do not inherit that replace, so pin
    # the SDK to the same commit as the server source. This keeps the parser and
    # its request types aligned across release tags (notably OpenBao 2.6+).
    GOFLAGS="${TMPMODULE_GOFLAGS}" go get \
      "github.com/openbao/openbao@${sha}" \
      "github.com/openbao/openbao/sdk/v2@${sha}" >/dev/null 2>&1
    GOFLAGS="${TMPMODULE_GOFLAGS}" go mod tidy >/dev/null 2>&1
    GOFLAGS="${TMPMODULE_GOFLAGS}" go run . "${FILES[@]}"

    if [ "${REPORT_SCHEMA_DRIFT}" = "true" ]; then
      moddir="$(GOFLAGS="${TMPMODULE_GOFLAGS}" go list -m -f '{{.Dir}}' github.com/openbao/openbao)"
      (
        cd "${ROOT_DIR}"
        GOFLAGS="${REPO_GOFLAGS}" go run ./hack/tools/openbao_config_schema -file "${moddir}/command/server/config.go"
      ) > "${schema_tmp}/keys-${version}.txt"
    fi
  )

  rm -rf "${tmpdir}"
  if [ "${REPORT_SCHEMA_DRIFT}" != "true" ]; then
    trap - EXIT
  fi
done

if [ "${REPORT_SCHEMA_DRIFT}" = "true" ]; then
  echo
  echo "==> OpenBao config schema drift report"
  echo "    Source: github.com/openbao/openbao/command/server/config.go (hcl struct tags)"
  echo

  any=false
  for ((i = 1; i < ${#VERSIONS[@]}; i++)); do
    prev="${VERSIONS[$((i-1))]}"
    cur="${VERSIONS[$i]}"
    prev_file="${schema_tmp}/keys-${prev}.txt"
    cur_file="${schema_tmp}/keys-${cur}.txt"

    if [ ! -f "${prev_file}" ] || [ ! -f "${cur_file}" ]; then
      echo "warning: missing schema snapshots for ${prev} -> ${cur}" >&2
      continue
    fi

    added="$(comm -13 "${prev_file}" "${cur_file}" || true)"
    removed="$(comm -23 "${prev_file}" "${cur_file}" || true)"

    if [ -n "${added}" ] || [ -n "${removed}" ]; then
      any=true
      echo "--- ${prev} -> ${cur}"
      if [ -n "${added}" ]; then
        echo "Added keys:"
        echo "${added}" | sed 's/^/  + /'
      fi
      if [ -n "${removed}" ]; then
        echo "Removed keys:"
        echo "${removed}" | sed 's/^/  - /'
      fi
      echo
    fi
  done

  if [ "${any}" = "false" ]; then
    echo "No config schema changes detected across: ${VERSIONS[*]}"
  fi

  if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
    {
      echo "### OpenBao config schema drift"
      echo
      echo "- Range: \`${VERSIONS[*]}\`"
      if [ "${any}" = "false" ]; then
        echo "- Result: no schema changes detected"
      else
        echo "- Result: schema changes detected (see job logs for details)"
      fi
    } >> "${GITHUB_STEP_SUMMARY}"
  fi
fi
