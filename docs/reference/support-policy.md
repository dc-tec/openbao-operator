---
description: Support and maintenance policy for OpenBao Operator covering pre-GA posture, release channels, and best-effort latest-line support.
---

# Support & Maintenance Policy

This document defines which OpenBao Operator releases are supported and what support level to expect.

## 1. Pre-GA Release Contract

The current stable release line is intended for real deployments, but OpenBao Operator remains pre-GA:

- The served CRD API is `openbao.org/v1alpha1`.
- Minor releases (`0.Y.0`) may introduce breaking API or behavior changes.
- Support is best-effort and focused on the latest stable release line.

See [Deprecation Policy](deprecation-policy.md) for the API evolution rules.

## 2. Release Channels

The Operator publishes multiple channels:

- **Stable** (`X.Y.Z`): intended for real deployments.
- **Prerelease** (`X.Y.Z-rc.1`, `X.Y.Z-beta.1`, `X.Y.Z-alpha.1`): early access builds for evaluation before the next stable release.
- **Edge** (`main` snapshots): continuous validation channel.
- **Nightly** (scheduled snapshots): scheduled validation channel.

Channel behavior and publication details are defined in [Release Management](../contributing/release-management.md).

## 3. Supported Versions

!!! note "Current support window"
    We provide **best-effort support** for the latest stable release line.

For `0.x`:

- Latest stable release line: eligible for issue triage and bug or security fixes.
- Older stable releases: no guarantee of backported fixes.
- Prereleases: evaluation builds for the next stable line; they do not expand the support window.
- Edge/Nightly: validation channels, not production support channels.

## 4. Validation Versus Support

Validation and support are related but different:

- [Compatibility Matrix](compatibility.md) defines what is explicitly validated in CI.
- This page defines what receives best-effort maintenance attention.
- `Recommended for production` means the documented hardened operating path, not a promise of a stable pre-GA API.

## 5. Security Fixes

Security fixes follow [SECURITY.md](https://github.com/dc-tec/openbao-operator/blob/main/SECURITY.md):

- Security fixes are provided for the latest released version.
- Report vulnerabilities via GitHub Security Advisories.

## 6. Compatibility Baseline

Supported Kubernetes and OpenBao versions are defined in:

- [Compatibility Matrix](compatibility.md)

If a platform/version is outside that matrix, it is out of support scope.

## 7. Support Expectations

Support is best-effort community support through repository workflows and issues.

- No formal SLA/SLO is provided for response or remediation timelines.
- Upgrade to the latest stable release before requesting issue triage.

## 8. Recommended Operations Policy

For production use:

1. Pin explicit operator/chart versions.
2. Stay close to latest stable.
3. Use the `Hardened` profile with admission enforcement enabled.
4. Validate upgrades in staging.
5. Avoid running production on prerelease, Edge, or Nightly builds.
