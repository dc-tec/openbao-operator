---
description: Prescriptive guide for choosing tenancy mode, security profile, bootstrap flow, TLS mode, admission posture, and upgrade strategy for OpenBao Operator.
slug: /get-started/deployment-decision-guide
---

# Deployment Decision Guide

Use this guide to choose the default operating path for OpenBao Operator. Start with the default production path unless you have a clear reason to deviate.

## Default Production Path

<Callout type="success" title="Start Here">

For most production deployments, use this combination:

- `tenancy.mode=multi`
- `spec.profile: Hardened`
- `spec.selfInit.enabled: true`
- `spec.tls.mode: External` or `ACME`
- `spec.upgrade.strategy: RollingUpdate`
- `admissionPolicies.enabled=true`

</Callout>

## Decision Matrix

| Decision | Default choice | Choose the alternative when | Tradeoff | Reference |
| :--- | :--- | :--- | :--- | :--- |
| **Tenancy model** | **Multi-Tenant** | Use **Single-Tenant** when one team owns one namespace and you intentionally want direct namespace-scoped operator control. | Single-tenant is simpler, but it does not use the default namespace-introduction model. | [Single-Tenant Mode](single-tenant-mode.md) |
| **Security profile** | **Hardened** | Use **Development** only for local testing, CI, or short-lived evaluation environments. | Development relaxes bootstrap and transport guardrails and is highly discouraged for production. | [Security Profiles](../openbaocluster/configuration/security-profiles.md) |
| **Bootstrap flow** | **Self-Init** | Use manual bootstrap only for compatibility or controlled break-glass workflows. | Manual bootstrap stores a root token Secret and is not a supported production path. | [Self-Initialization](../openbaocluster/configuration/self-init.md) |
| **TLS mode** | **External** or **ACME** | Use `OperatorManaged` only in non-Hardened environments where internal PKI convenience matters more than production trust requirements. | `OperatorManaged` TLS is rejected for Hardened clusters and is not a production path there. | [TLS & Identity](../../security/workload/tls.md) |
| **Installation path** | **Helm** | Use raw manifests when you need install-time identity customization, local overlay control, or source-based rendering. | Helm is easier to keep consistent. Raw manifests require you to choose the right overlay and verify rendered identities. | [Operator Installation](installation.md) |
| **Admission posture** | **Admission policies enabled** | Disable admission policies only for development or break-glass recovery. | Disabling them enters unsafe mode and removes part of the normal safety model. | [Admission Policies](../../security/infrastructure/admission-policies.md) |
| **Upgrade strategy** | **RollingUpdate** | Use **BlueGreen** when you need parallel validation, manual promotion, or stronger cutover boundaries. | Blue/green uses more resources and adds operational complexity. | [Cluster Upgrades](../openbaocluster/operations/upgrades.md) |
| **Platform mode** | **Auto-detection** | Use explicit OpenShift mode when the target cluster is OpenShift or when SCC-compatible rendering must be explicit. | Explicit platform selection gives you predictable rendering, but you still need target-cluster validation for the exact SCC and platform behavior you rely on. | [Operator Installation](installation.md) |

## Recommended Profiles

<Tabs groupId="shared-production-dedicated-team-namespace-local-development-or-ci">

<TabItem value="shared-production" label="Shared Production">

Use this profile for the normal production path:

- `tenancy.mode=multi`
- `spec.profile: Hardened`
- `spec.selfInit.enabled: true`
- `spec.tls.mode: External` or `ACME`
- `spec.upgrade.strategy: RollingUpdate`
- Scheduled backups configured before the first production upgrade

</TabItem>

<TabItem value="dedicated-team-namespace" label="Dedicated Team Namespace">

Use this profile when one team owns one namespace and does not need the default tenant-onboarding model:

- `tenancy.mode=single`
- `spec.profile: Hardened`
- `spec.selfInit.enabled: true`
- `spec.tls.mode: External` or `ACME`
- `spec.upgrade.strategy: RollingUpdate`
- Admission policies still enabled

</TabItem>

<TabItem value="local-development-or-ci" label="Local Development or CI">

Use this profile only for non-production environments:

- `tenancy.mode=multi` or `single`, depending on the scenario under test
- `spec.profile: Development`
- `spec.tls.mode: OperatorManaged` is acceptable
- Manual bootstrap is acceptable if you need the root token Secret
- Unsafe mode only when you are intentionally testing without admission enforcement

</TabItem>

</Tabs>

## Operational Checks

Before calling a deployment production-ready, verify these choices:

1. `Hardened` profile is selected.
2. `selfInit.enabled` is `true`.
3. TLS mode is `External` or `ACME`.
4. Admission policies are installed and enforced.
5. Backups are configured and tested.
6. Upgrade validation is done in staging, with `RollingUpdate` as the default strategy unless you need blue/green cutover control.

## Status Checkpoints

Use these condition checkpoints before calling a path ready:

| Path | Minimum checkpoint |
| :--- | :--- |
| Hardened with External TLS | `Available=True`, `TLSReady=True`, `UserAccessBootstrap=True`, `ProductionReady=True` |
| Hardened with ACME | `Available=True`, `ACMEIntegrationReady=True`, `ACMECacheReady=True`, `UserAccessBootstrap=True`, `ProductionReady=True` |
| Gateway exposure | `GatewayIntegrationReady=True` |
| Strict NetworkPolicy clusters | `APIServerNetworkReady=True` or `Unknown` with `APIServerEndpointIPsRecommended` after you confirm the service-VIP path works in your CNI |
| Scheduled backups | `BackupConfigurationReady=True` |
| Restore before execution | `RestoreConfigurationReady=True` |

## See Also

- [Operator Invariants](../../architecture/operator-invariants.md)
- [Production Checklist](../openbaocluster/operations/production-checklist.md)
- [Multi-Tenancy](../openbaotenant/multi-tenancy.md)
