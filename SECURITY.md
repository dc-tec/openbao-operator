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

For trusted root maintenance used by keyless verification, see `docs/contribute/release-management.md`.

## Known dependency caveats

In this operator, we **pin** Sigstore trusted root material (either embedded `internal/adapter/security/trusted_root.json` or a user-provided ConfigMap) and pass it to Cosign for both keyless verification and Rekor transparency log verification. This avoids fetching/updating trusted root data via TUF at runtime. The only TUF fetch logic in this repository is `internal/adapter/security/fetch_trusted_root.go`, which is build-ignored and intended for maintainers to refresh the pinned `trusted_root.json`.

Govulncheck ignores `GO-2026-5932` for now. The finding is inherited through the image verification path `github.com/sigstore/cosign/v3` -> `github.com/sigstore/rekor/pkg/pki/pgp` -> `golang.org/x/crypto/openpgp`; it is reached while verifying container image signatures and Rekor transparency log entries. The Go advisory has no fixed `golang.org/x/crypto` version, and `github.com/sigstore/cosign/v3` is already pinned to the latest available v3 release at the time of this change. Remove the ignore once Sigstore/Rekor publishes a remediation that avoids `golang.org/x/crypto/openpgp` or the Go advisory gains a fixed version.
