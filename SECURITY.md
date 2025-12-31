# Security Policy

## Reporting a Vulnerability

Please do **not** open public GitHub issues for security-sensitive reports.

Instead, use GitHub Security Advisories:

- https://github.com/dc-tec/openbao-operator/security/advisories/new

If you are unable to use GitHub Security Advisories, open a minimal issue that requests a private contact channel and do not include exploit details.

## Supported Versions

Security fixes are provided for:

- The latest released version.

## Supply Chain

Release artifacts (container images and OCI Helm chart) are published to GHCR and are signed using keyless Sigstore signing via GitHub Actions OIDC.

For trusted root maintenance used by keyless verification, see `docs/contributing/release-management.md`.
