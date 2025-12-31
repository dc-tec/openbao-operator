# CI

This document describes how CI is structured and how to run CI-equivalent checks locally.

## CI Overview

GitHub Actions is the source of truth for CI:

- PR and `main` pushes run `.github/workflows/ci.yml`.
- Nightly runs `.github/workflows/nightly.yml` (full E2E matrix).

The CI jobs are designed to be deterministic:

- Dependency drift is checked explicitly (`verify-tidy`), tests run with `GOFLAGS=-mod=readonly`.
- Kind is installed from a pinned release and checksum-verified.
- E2E uses pinned `kindest/node` images to lock Kubernetes versions.

## What Runs Where

### PRs / `main` pushes (`.github/workflows/ci.yml`)

- `make lint-config lint`
- `make verify-fmt`
- `make verify-tidy`
- `make verify-generated`
- `make test-ci` (unit + envtest)
- Docs build (MkDocs container)
- Helm chart validation (`helm lint`, `helm template --include-crds`)
- Security checks:
  - `govulncheck -test ./...`
  - Trivy filesystem scan (vuln + config, HIGH/CRITICAL only)
- E2E smoke on kind v1.34.x (label filter `smoke`)

### Nightly (`.github/workflows/nightly.yml`)

- Full E2E suite on a Kubernetes matrix from `docs/reference/compatibility.md`
- Security checks (same as PRs)
- Docs build, Helm chart validation

## Preparing Your Local Environment

### Required

- Go: match the version in `go.mod`
- Docker: required for E2E (kind) and docs builds
- `kubectl`
- `make`

### Recommended

- `kind` (CI installs it automatically)
- `trivy` (for local security scans)

## Running CI Locally

Run the same checks CI runs for PRs:

```sh
make lint-config lint
make verify-fmt
make verify-tidy
make verify-generated
make test-ci
```

## Security Checks Locally

Run Go vulnerability scanning:

```sh
go install golang.org/x/vuln/cmd/govulncheck@v1.1.4
govulncheck -test ./...
```

Run Trivy scans (requires `trivy` installed):

```sh
make security-scan IMG=ghcr.io/dc-tec/openbao-operator:vX.Y.Z
```

## E2E (kind)

### Smoke tests (PR-equivalent)

```sh
make test-e2e-ci \
  KIND_NODE_IMAGE=kindest/node:v1.34.3 \
  E2E_LABEL_FILTER=smoke \
  E2E_PARALLEL_NODES=1
```

### Full suite (nightly-equivalent)

```sh
make test-e2e-ci KIND_NODE_IMAGE=kindest/node:v1.34.3
```

### Debugging

Keep the cluster after failures:

```sh
make test-e2e-ci E2E_SKIP_CLEANUP=true
```

## Docs Build and Publishing

### Build docs locally (CI-equivalent)

CI builds docs using a pinned `mkdocs-material` container image:

```sh
docker run --rm \
  -v "${PWD}:/docs" \
  -w /docs \
  squidfunk/mkdocs-material@sha256:3bba0a99bc6e635bb8e53f379d32ab9cecb554adee9cc8f59a347f93ecf82f3b \
  build -s
```

This writes the static site to `./site/`.

### GitHub Pages deployment

On pushes to `main`, `.github/workflows/pages.yml` deploys the contents of `site/` to GitHub Pages.

Repository settings must be configured once:

- Settings → Pages → Source: GitHub Actions

The expected public URL is configured in `mkdocs.yml` as `site_url`.
