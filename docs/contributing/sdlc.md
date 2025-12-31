# SDLC Overview

This page maps the OpenBao Operator SDLC (software development lifecycle) to concrete repository artifacts.

## Plan

- Compatibility policy: `docs/reference/compatibility.md`

## Design

- Architecture overview: `docs/architecture/index.md`
- Component docs: `docs/architecture/components.md`

## Implement

- Coding standards: `AGENTS.md`
- Development setup: `docs/contributing/getting-started/development.md`

## Review

- Pull requests are expected for all changes (see `.github/pull_request_template.md`).
- Issues use structured templates for triage (`.github/ISSUE_TEMPLATE/`).

## Verify

- Unit + envtest: `make test-ci`
- E2E smoke on PRs: `.github/workflows/ci.yml`
- Full E2E nightly: `.github/workflows/nightly.yml`
- Helm validation in CI: `.github/workflows/ci.yml`

## Secure

- Static analysis and linting: `make lint-config lint`
- Dependency updates: `.github/dependabot.yml`
- Dependency regression gate: `.github/workflows/dependency-review.yml`
- Go vuln scanning: `govulncheck` (CI and release gates)
- Filesystem + config scanning: Trivy (CI and nightly)
- Keyless signing for release artifacts: `.github/workflows/release.yml`
- Build provenance attestations: `.github/workflows/release.yml`
- Sigstore trusted root maintenance: `.github/workflows/trusted-root-refresh.yml`

## Release

- Release process: `docs/contributing/release-management.md`
- Release workflow: `.github/workflows/release.yml`
- Release assets:
  - digest-pinned `dist/install.yaml`
  - per-image SBOMs (`dist/sbom-*.spdx.json`)

## Deploy

- Installer YAML: GitHub Release assets (`dist/install.yaml`)
- Helm chart (OCI): `charts/openbao-operator/`

## Operate

- Runbooks and recovery docs: `docs/user-guide/`
- Security posture and multi-tenancy: `docs/security/`

## Improve

- CI and local workflows: `docs/contributing/ci.md`
