---
description: Support and maintenance policy for OpenBao Operator release channels, version support window, and security fix expectations.
---

# Support & Maintenance Policy

This document defines which OpenBao Operator releases are supported and what support level to expect.

## 1. Release Channels

The Operator publishes multiple channels:

- **Stable / SemVer** (`X.Y.Z`, including prereleases)
- **Edge** (`main` snapshots)
- **Nightly** (scheduled snapshots)

Channel behavior and publication details are defined in [Release Management](../contributing/release-management.md).

## 2. Supported Versions

!!! note "Current support window"
    We provide support for the **latest stable release line**.

For `0.x`:

- Latest stable release: supported for bug and security fixes.
- Older stable releases: no guarantee of backported fixes.
- Edge/Nightly: validation channels, not production support channels.

## 3. Security Fixes

Security fixes follow [SECURITY.md](https://github.com/dc-tec/openbao-operator/blob/main/SECURITY.md):

- Security fixes are provided for the latest released version.
- Report vulnerabilities via GitHub Security Advisories.

## 4. Compatibility Baseline

Supported Kubernetes and OpenBao versions are defined in:

- [Compatibility Matrix](compatibility.md)

If a platform/version is outside that matrix, it is out of support scope.

## 5. Support Expectations

Support is best-effort community support through repository workflows and issues.

- No formal SLA/SLO is provided for response or remediation timelines.
- Upgrade to the latest stable release before requesting issue triage.

## 6. Recommended Operations Policy

For production use:

1. Pin explicit operator/chart versions.
2. Stay close to latest stable.
3. Validate upgrades in staging.
4. Avoid running production on Edge/Nightly.
