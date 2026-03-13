---
description: Compatibility matrix for OpenBao Operator covering CI validation scope, support posture, and production guidance.
---

# Compatibility Matrix

This page defines the validation coverage, support posture, and production guidance for OpenBao Operator.

!!! note "Terminology"
    Use these terms consistently on this page:

    - **Validated**: explicitly exercised in CI.
    - **Best-effort supported**: eligible for issue triage and fixes on the latest stable release line only.
    - **Recommended for production**: stable releases using the documented `Hardened` profile, `selfInit`, admission enforcement, explicit version pinning, and staged upgrade validation.

## 1. Pre-GA Posture

The current stable release line is intended for real deployments, but it remains pre-GA. The served CRD API is `openbao.org/v1alpha1`, and minor releases may introduce breaking changes. See [Deprecation Policy](deprecation-policy.md) and [Support & Maintenance](support-policy.md) for the full release contract.

## 2. Kubernetes Versions

We run full nightly E2E coverage across the listed Kubernetes versions.

| Version | Validated | Support posture | Production note |
| :--- | :--- | :--- | :--- |
| **v1.35** | PR gate plus nightly E2E | Best-effort supported on the latest stable line | Primary validated baseline |
| **v1.34** | Nightly E2E | Best-effort supported on the latest stable line | Recommended after staging validation |
| **v1.33** | Nightly E2E | Best-effort supported on the latest stable line | Minimum validated version |
| **v1.32** | Not validated | Out of support scope | Do not use for the current pre-GA line |

## 3. Platforms

| Platform | Validated | Support posture | Production note |
| :--- | :--- | :--- | :--- |
| **Kubernetes** | Default CI and nightly path | Best-effort supported on the latest stable line | Default recommended platform path |
| **OpenShift** | Helm rendering regression checks, manifest admission tests, and focused platform E2E | Best-effort supported on the latest stable line | Validate on your target cluster before production rollout |

## 4. OpenBao Versions

| Version | Validated | Support posture | Production note |
| :--- | :--- | :--- | :--- |
| **2.5.x** | PR gate, nightly E2E, and config compatibility checks | Best-effort supported on the latest stable line | Primary validated target |
| **2.4.x** | Nightly config compatibility checks | Best-effort supported on the latest stable line | Validate workload behavior and upgrade flow in staging before production rollout |
| **2.3.x** | Not validated | Deprecated and out of support scope | Upgrade before production use |

## 5. CI Validation Matrix

We treat the CI configuration as the source of truth for validation coverage.

| Workflow | Scope | Versions Tested |
| :--- | :--- | :--- |
| **PR Gate** | Logic, manifests, and primary compatibility path | K8s `1.35` + OpenBao `2.5.0` |
| **Nightly E2E** | Full lifecycle coverage | K8s `1.33`, `1.34`, `1.35` + OpenBao `2.5.0` |
| **Nightly Config Compatibility** | Render and config compatibility checks | OpenBao `2.4.4` and `2.5.0` |

!!! warning "Production Upgrade"
    Always validate new Kubernetes or OpenBao versions in a staging environment before upgrading production, even when they are listed as validated and best-effort supported.
