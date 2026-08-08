#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SITE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
LEDGER="${SITE_DIR}/redirects/legacy_redirects.tsv"
DESTINATION=""
LEGACY_BUILD=""
MODE="write"

usage() {
  cat >&2 <<'EOF'
usage: apply-legacy-redirects.sh --destination DIR [--legacy-build DIR] [--check]

Writes the redirect pages declared in redirects/legacy_redirects.tsv into an existing
Hugo build. --check verifies an already processed build without changing it.
--legacy-build additionally proves that the ledger accounts for every route in
the retained Docusaurus build used for the migration.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --destination)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      DESTINATION="$2"
      shift
      ;;
    --legacy-build)
      [[ $# -ge 2 ]] || { usage; exit 2; }
      LEGACY_BUILD="$2"
      shift
      ;;
    --check)
      MODE="check"
      ;;
    *)
      usage
      exit 2
      ;;
  esac
  shift
done

[[ -n "${DESTINATION}" ]] || { usage; exit 2; }
[[ -d "${DESTINATION}" ]] || { echo "Hugo destination not found: ${DESTINATION}" >&2; exit 1; }
[[ -f "${LEDGER}" ]] || { echo "legacy redirect ledger not found: ${LEDGER}" >&2; exit 1; }
if [[ -n "${LEGACY_BUILD}" && ! -d "${LEGACY_BUILD}" ]]; then
  echo "legacy Docusaurus build not found: ${LEGACY_BUILD}" >&2
  exit 1
fi

base_url="$(awk -F '"' '/^baseURL = / { print $2; exit }' "${SITE_DIR}/hugo.toml")"
[[ -n "${base_url}" ]] || { echo "baseURL not found in ${SITE_DIR}/hugo.toml" >&2; exit 1; }
host_path="${base_url#*://}"
base_path="/${host_path#*/}"
base_path="${base_path%/}"
base_url="${base_url%/}"

work_dir="$(mktemp -d)"
trap 'rm -rf "${work_dir}"' EXIT
ledger_routes="${work_dir}/ledger-routes"
preserved_routes="${work_dir}/preserved-routes"
redirect_routes="${work_dir}/redirect-routes"
legacy_routes="${work_dir}/legacy-routes"
: > "${ledger_routes}"
: > "${preserved_routes}"
: > "${redirect_routes}"

declare -A seen_routes=()
entries=0
preserved=0
redirects=0
missing_targets=0
invalid_entries=0
stale_redirects=0
redirect_chains=0

route_file() {
  local root="$1"
  local route="$2"
  if [[ "${route}" == "/" ]]; then
    printf '%s/index.html' "${root%/}"
  else
    printf '%s/%sindex.html' "${root%/}" "${route#/}"
  fi
}

render_redirect() {
  local source="$1"
  local target="$2"
  local output="$3"
  local target_path="${base_path}${target}"
  local canonical="${base_url}${target}"

  cat > "${output}" <<EOF
<!doctype html>
<html lang="en-US">
  <head>
    <meta charset="utf-8">
    <meta name="robots" content="noindex">
    <meta http-equiv="refresh" content="0; url=${target_path}">
    <link rel="canonical" href="${canonical}">
    <title>Redirecting to OpenBao Operator documentation</title>
    <script>window.location.replace("${target_path}" + window.location.search + window.location.hash);</script>
  </head>
  <body>
    <p>This documentation moved to <a href="${target_path}">${target_path}</a>.</p>
  </body>
</html>
EOF
}

while IFS=$'\t' read -r source target _evidence; do
  [[ -n "${source}" ]] || continue
  [[ "${source}" == \#* ]] && continue
  entries=$((entries + 1))

  if [[ ( "${source}" != "/" && ! "${source}" =~ ^/[A-Za-z0-9._~/-]*/$ ) || ( "${target}" != "/" && ! "${target}" =~ ^/[A-Za-z0-9._~/-]*/$ ) || "${source}" == *".."* || "${target}" == *".."* ]]; then
    echo "invalid legacy redirect entry at line ${entries}: ${source} -> ${target}" >&2
    invalid_entries=$((invalid_entries + 1))
    continue
  fi
  if [[ -n "${seen_routes[${source}]:-}" ]]; then
    echo "duplicate legacy route: ${source}" >&2
    invalid_entries=$((invalid_entries + 1))
    continue
  fi
  seen_routes["${source}"]=1
  printf '%s\n' "${source}" >> "${ledger_routes}"

  target_file="$(route_file "${DESTINATION}" "${target}")"
  if [[ ! -f "${target_file}" ]]; then
    echo "legacy redirect target does not exist: ${source} -> ${target}" >&2
    missing_targets=$((missing_targets + 1))
    continue
  fi

  if [[ "${source}" == "${target}" ]]; then
    preserved=$((preserved + 1))
    printf '%s\n' "${source}" >> "${preserved_routes}"
    continue
  fi

  redirects=$((redirects + 1))
  printf '%s\n' "${source}" >> "${redirect_routes}"
  source_file="$(route_file "${DESTINATION}" "${source}")"
  expected_file="${work_dir}/redirect-${redirects}.html"
  render_redirect "${source}" "${target}" "${expected_file}"

  if [[ "${MODE}" == "check" ]]; then
    if [[ ! -f "${source_file}" ]] || ! cmp -s "${expected_file}" "${source_file}"; then
      echo "legacy redirect is missing or stale: ${source} -> ${target}" >&2
      stale_redirects=$((stale_redirects + 1))
    fi
  else
    mkdir -p "$(dirname "${source_file}")"
    cp "${expected_file}" "${source_file}"
  fi
done < "${LEDGER}"

sort -u -o "${ledger_routes}" "${ledger_routes}"
sort -u -o "${preserved_routes}" "${preserved_routes}"
sort -u -o "${redirect_routes}" "${redirect_routes}"
unmapped=0
additional=0
legacy_preserved=0
legacy_redirects=0
if [[ -n "${LEGACY_BUILD}" ]]; then
  while IFS= read -r path; do
    route="/${path#"${LEGACY_BUILD%/}"/}"
    route="${route%index.html}"
    printf '%s\n' "${route}"
  done < <(find "${LEGACY_BUILD}" -type f -name index.html | sort) > "${legacy_routes}"
  sort -u -o "${legacy_routes}" "${legacy_routes}"

  unmapped="$(comm -23 "${legacy_routes}" "${ledger_routes}" | tee "${work_dir}/unmapped" | wc -l | tr -d ' ')"
  additional="$(comm -13 "${legacy_routes}" "${ledger_routes}" | wc -l | tr -d ' ')"
  legacy_preserved="$(comm -12 "${legacy_routes}" "${preserved_routes}" | wc -l | tr -d ' ')"
  legacy_redirects="$(comm -12 "${legacy_routes}" "${redirect_routes}" | wc -l | tr -d ' ')"
  if [[ "${unmapped}" -gt 0 ]]; then
    echo "legacy routes missing from the ledger:" >&2
    sed 's/^/  /' "${work_dir}/unmapped" >&2
  fi
fi

# A redirect target must remain a canonical Hugo page. Checking after all writes
# catches a ledger entry that would otherwise overwrite another entry's target.
while IFS=$'\t' read -r source target _evidence; do
  [[ -n "${source}" ]] || continue
  [[ "${source}" == \#* || "${source}" == "${target}" ]] && continue
  target_file="$(route_file "${DESTINATION}" "${target}")"
  if grep -qF 'Redirecting to OpenBao Operator documentation' "${target_file}"; then
    echo "legacy redirect chain detected: ${source} -> ${target}" >&2
    redirect_chains=$((redirect_chains + 1))
  fi
done < "${LEDGER}"

printf 'Ledger routes: %d; preserved: %d; redirects: %d; missing targets: %d; stale redirects: %d; redirect chains: %d\n' \
  "${entries}" "${preserved}" "${redirects}" "${missing_targets}" "${stale_redirects}" "${redirect_chains}"
if [[ -n "${LEGACY_BUILD}" ]]; then
  printf 'Docusaurus routes: %d; preserved: %d; mapped: %d; unmapped: %d; additional compatibility routes: %d\n' \
    "$(wc -l < "${legacy_routes}" | tr -d ' ')" "${legacy_preserved}" "${legacy_redirects}" "${unmapped}" "${additional}"
fi

if [[ "${invalid_entries}" -gt 0 || "${missing_targets}" -gt 0 || "${stale_redirects}" -gt 0 || "${redirect_chains}" -gt 0 || "${unmapped}" -gt 0 ]]; then
  exit 1
fi
