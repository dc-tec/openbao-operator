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

## Known dependency caveats

`govulncheck` may report three vulnerabilities in the transitive dependency `github.com/theupdateframework/go-tuf` v0.7.0:

- **GO-2026-4377** – Path traversal in TAP 4 multirepo client (CVE-2026-24686)
- **GO-2026-4349** – Improper validation of delegation threshold (CVE-2026-23992)
- **GO-2026-4348** – Client DoS via malformed server response (CVE-2026-23991)

This package is not imported directly; it is pulled in transitively by the Sigstore stack (cosign, sigstore-go, sigstore, rekor) used for container image signature verification. The go-tuf v0 module has **no fixed version** in the Go vulnerability database; the v0 line is effectively unpatched. The project already uses the patched `go-tuf/v2` v2.4.1 where applicable.

In this operator, we **pin** Sigstore trusted root material (either embedded `internal/adapter/security/trusted_root.json` or a user-provided ConfigMap) and pass it to Cosign for both keyless verification and Rekor transparency log verification. This avoids fetching/updating trusted root data via TUF at runtime. The only TUF fetch logic in this repository is `internal/adapter/security/fetch_trusted_root.go`, which is build-ignored and intended for maintainers to refresh the pinned `trusted_root.json`.

We are tracking upstream (e.g. [sigstore/cosign](https://github.com/sigstore/cosign), [sigstore/sigstore-go](https://github.com/sigstore/sigstore-go)) to drop the legacy go-tuf v0 dependency in favor of go-tuf/v2. Until then, these findings are accepted as an upstream dependency limitation and are listed in `.govulnignore` so that `make vulncheck` does not fail. Re-run `govulncheck ./...` after upgrading cosign/sigstore-go to see if newer releases have removed the v0 dependency; you can then remove the corresponding entries from `.govulnignore`.
