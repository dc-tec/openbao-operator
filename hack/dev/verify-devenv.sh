#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

# shellcheck source=/dev/null
source "${ROOT_DIR}/hack/dev/tool-versions.env"

fail() {
  printf 'ERR  %s\n' "$1" >&2
  exit 1
}

ok() {
  printf 'OK   %s\n' "$1"
}

require_nix_command() {
  local name="$1"
  local command_path=""
  local resolved_path=""

  command_path="$(command -v "${name}" 2>/dev/null || true)"
  if [[ -z "${command_path}" ]]; then
    fail "${name} is missing from the devenv environment"
  fi

  resolved_path="$(realpath "${command_path}")"
  if [[ "${resolved_path}" != /nix/store/* ]]; then
    fail "${name} resolved outside the Nix store: ${resolved_path}"
  fi

  ok "${name}: ${resolved_path}"
}

if [[ -z "${DEVENV_ROOT:-}" ]]; then
  fail "DEVENV_ROOT is not set; run this check through 'devenv test' or 'devenv shell'"
fi

devenv_root="$(cd "${DEVENV_ROOT}" && pwd)"
if [[ "${devenv_root}" != "${ROOT_DIR}" ]]; then
  fail "DEVENV_ROOT points to ${devenv_root}, expected ${ROOT_DIR}"
fi

if [[ -z "${DEVENV_PROFILE:-}" ]]; then
  fail "DEVENV_PROFILE is not set inside devenv"
fi
if [[ "${PATH%%:*}" != "${DEVENV_PROFILE}/bin" ]]; then
  fail "the active devenv profile must take precedence in PATH"
fi
ok "active devenv profile takes precedence in PATH"

if [[ "${GOTOOLCHAIN:-}" != "local" ]]; then
  fail "GOTOOLCHAIN must be local inside devenv, found ${GOTOOLCHAIN:-unset}"
fi

if [[ "${GOROOT:-}" != /nix/store/*/share/go ]]; then
  fail "GOROOT must point to the pinned Nix Go package, found ${GOROOT:-unset}"
fi
ok "GOROOT points to the pinned Nix Go package (${GOROOT})"

if [[ -z "${GOPATH:-}" ]]; then
  fail "GOPATH is not set inside devenv"
fi
if [[ "${GOPATH}" == "${ROOT_DIR}" || "${GOPATH}" == "${ROOT_DIR}/"* ]]; then
  fail "GOPATH must stay outside the repository so generators do not scan its module cache: ${GOPATH}"
fi
ok "GOPATH is outside the repository (${GOPATH})"

expected_go="$(sed -n 's/^go //p' go.mod | head -n 1)"
current_go="$(go env GOVERSION 2>/dev/null || true)"
if [[ "${current_go}" != "go${expected_go}" ]]; then
  fail "Go toolchain mismatch: expected go${expected_go}, found ${current_go:-unknown}"
fi
ok "Go ${expected_go} matches go.mod"

expected_hugo="$(tr -d '[:space:]' < .hugo-version)"
current_hugo="$(hugo version 2>/dev/null || true)"
if [[ "${current_hugo}" != *"v${expected_hugo}"* ]]; then
  fail "Hugo mismatch: expected ${expected_hugo}, found ${current_hugo:-unknown}"
fi
ok "Hugo ${expected_hugo} matches .hugo-version"

current_helm="$(helm version --short 2>/dev/null || true)"
if [[ "${current_helm}" != "${HELM_VERSION}" && "${current_helm}" != "${HELM_VERSION}"+* ]]; then
  fail "Helm mismatch: expected ${HELM_VERSION}, found ${current_helm:-unknown}"
fi
ok "Helm matches tool-versions.env (${current_helm})"

current_kubectl="$(kubectl version --client --output=json 2>/dev/null | jq -r '.clientVersion.gitVersion // empty')"
if [[ "${current_kubectl}" != "${KUBECTL_VERSION}" ]]; then
  fail "kubectl mismatch: expected ${KUBECTL_VERSION}, found ${current_kubectl:-unknown}"
fi
ok "kubectl matches tool-versions.env (${current_kubectl})"

current_kind="$(kind version 2>/dev/null || true)"
if [[ "${current_kind}" != *"${KIND_VERSION}"* ]]; then
  fail "Kind mismatch: expected ${KIND_VERSION}, found ${current_kind:-unknown}"
fi
ok "Kind matches tool-versions.env (${current_kind})"

current_trivy="$(trivy version 2>/dev/null | sed -n 's/^Version: //p' | head -n 1)"
if [[ "v${current_trivy}" != "${TRIVY_VERSION}" ]]; then
  fail "Trivy mismatch: expected ${TRIVY_VERSION}, found ${current_trivy:-unknown}"
fi
ok "Trivy matches tool-versions.env (v${current_trivy})"

current_tilt="$(tilt version 2>/dev/null | sed -n 's/^v\([^,]*\).*/v\1/p' | head -n 1)"
if [[ "${current_tilt}" != "${TILT_VERSION}" ]]; then
  fail "Tilt mismatch: expected ${TILT_VERSION}, found ${current_tilt:-unknown}"
fi
ok "Tilt matches tool-versions.env (${current_tilt})"

for command_name in \
  bash curl docker git go helm hugo jq kind kubectl make python3 tilt trivy; do
  require_nix_command "${command_name}"
done

printf '\nPinned devenv toolchain contract verified.\n'
