#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "${work_dir}"' EXIT
mkdir -p "${work_dir}/bin" "${work_dir}/dist"
cp "$1" "${work_dir}/dist/"
export OWNER=dc-tec
export GITHUB_OUTPUT="${work_dir}/output"
export TEST_PACKAGE="${work_dir}/dist/openbao-operator-${CHART_VERSION}.tgz"
export TEST_LOG="${work_dir}/commands"
export TEST_REGISTRY="${work_dir}/registry.tgz"

cat > "${work_dir}/bin/helm" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf 'helm %s\n' "$*" >> "${TEST_LOG}"
case "$1" in
  pull)
    reference="$2"
    if [[ "${TEST_PULL_FAILURE:-}" != "" ]]; then
      echo "${TEST_PULL_FAILURE}" >&2
      exit 1
    fi
    if [[ ! -f "${TEST_REGISTRY}" ]]; then
      echo 'chart: not found' >&2
      exit 1
    fi
    filename="$(basename "${TEST_PACKAGE}")"
    if [[ "${reference}" == *@sha256:* ]]; then
      # Helm derives the archive filename from the OCI reference, including its digest.
      filename="${reference##*/}"
      filename="${filename%:*}-${filename##*:}.tgz"
    fi
    while [[ "$1" != --destination ]]; do shift; done
    if [[ "${reference}" == *@sha256:* ]]; then
      case "${TEST_DIGEST_PULL_RESULT:-valid}" in
        missing) exit 0 ;;
        multiple) cp "${TEST_REGISTRY}" "$2/extra.tgz" ;;
        different)
          printf 'different digest archive\n' > "$2/${filename}"
          exit 0
          ;;
      esac
    fi
    cp "${TEST_REGISTRY}" "$2/${filename}"
    ;;
  push)
    [[ "$3" == oci://ghcr.io/dc-tec/charts-edge ]]
    cp "$2" "${TEST_REGISTRY}"
    ;;
  *) exit 1 ;;
esac
MOCK
cat > "${work_dir}/bin/docker" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf 'docker %s\n' "$*" >> "${TEST_LOG}"
if [[ "$3" == inspect ]]; then
  printf '"sha256:%064d"\n' 9
fi
MOCK
chmod +x "${work_dir}/bin/helm" "${work_dir}/bin/docker"
export PATH="${work_dir}/bin:${PATH}"
cd "${work_dir}"
printf 'installer\n' > dist/install.yaml
printf 'crds\n' > dist/crds.yaml
(cd dist && sha256sum install.yaml crds.yaml "openbao-operator-${CHART_VERSION}.tgz" > checksums.txt)

bash "${ROOT_DIR}/hack/ci/publish-edge-chart.sh"
[[ "$(grep -c 'edge-chart-' "${TEST_LOG}")" == 4 ]]
grep -q '^chart_digest=sha256:' "${GITHUB_OUTPUT}"
: > "${TEST_LOG}"
bash "${ROOT_DIR}/hack/ci/publish-edge-chart.sh"
if grep -Eq 'helm push|docker buildx imagetools create' "${TEST_LOG}"; then
  echo 'publisher rerun mutated an existing candidate' >&2
  exit 1
fi

# A successful version pull must not mask a missing, ambiguous, or mismatched digest archive.
for result in missing multiple different; do
  : > "${GITHUB_OUTPUT}"
  : > "${TEST_LOG}"
  if TEST_DIGEST_PULL_RESULT="${result}" bash "${ROOT_DIR}/hack/ci/publish-edge-chart.sh" >"${work_dir}/publish.err" 2>&1; then
    echo "publisher accepted a ${result} digest archive on rerun" >&2
    exit 1
  fi
  case "${result}" in
    missing) grep -q 'expected exactly one chart archive from digest pull, found 0' "${work_dir}/publish.err" ;;
    multiple) grep -q 'expected exactly one chart archive from digest pull, found 2' "${work_dir}/publish.err" ;;
    different) grep -q 'differ' "${work_dir}/publish.err" ;;
  esac
  test ! -s "${GITHUB_OUTPUT}"
  if grep -Eq 'helm push|docker buildx imagetools create' "${TEST_LOG}"; then
    echo 'failed publisher rerun mutated an existing candidate' >&2
    exit 1
  fi
done

if CHART_VERSION=0.5.0 bash "${ROOT_DIR}/hack/ci/publish-edge-chart.sh" >/dev/null 2>&1; then
  echo 'publisher accepted a release chart version' >&2
  exit 1
fi
if TEST_PULL_FAILURE='unauthorized' bash "${ROOT_DIR}/hack/ci/publish-edge-chart.sh" >/dev/null 2>&1; then
  echo 'publisher ignored a registry authentication failure' >&2
  exit 1
fi
printf 'different artifact\n' > "${TEST_REGISTRY}"
if bash "${ROOT_DIR}/hack/ci/publish-edge-chart.sh" >/dev/null 2>&1; then
  echo 'publisher overwrote an existing chart with different bytes' >&2
  exit 1
fi
