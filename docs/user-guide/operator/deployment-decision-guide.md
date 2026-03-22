---
title: Deployment Decision Guide
description: Prescriptive guide for choosing tenancy mode, security profile, bootstrap flow, TLS mode, admission posture, and upgrade strategy for OpenBao Operator.
slug: /get-started/deployment-decision-guide
hide_title: true
---

<JourneyHero
  eyebrow="Step 1"
  title="Choose the deployment path you want to keep operating."
  lede="Use this guide to lock down the default operating model before you install anything. Start with the production path unless you have a specific reason to trade off simplicity, isolation, or security posture."
  actions={[
    {label: 'Continue to installation', docId: 'user-guide/operator/installation', variant: 'primary'},
    {
      label: 'Review single-tenant mode',
      docId: 'user-guide/operator/single-tenant-mode',
      variant: 'secondary',
    },
  ]}
>
  <Checklist
    title="Default production path"
    items={[
      'tenancy.mode=multi',
      'spec.profile: Hardened',
      'spec.selfInit.enabled: true',
      'spec.tls.mode: External or ACME',
      'spec.upgrade.strategy: RollingUpdate',
      'admissionPolicies.enabled=true',
    ]}
    tone="success"
  />
</JourneyHero>

<JourneySteps
  title="Make the major decisions before you install"
  current={1}
  items={[
    {
      label: 'Choose a deployment path',
      description: 'Decide tenancy mode, security profile, TLS posture, and install method.',
      docId: 'user-guide/operator/deployment-decision-guide',
    },
    {
      label: 'Install the operator',
      description: 'Use Helm or manifests with the right namespace, identity, and admission model.',
      docId: 'user-guide/operator/installation',
    },
    {
      label: 'Create your first cluster',
      description: 'Apply a starting profile that matches local evaluation or hardened production.',
      docId: 'user-guide/openbaocluster/getting-started',
    },
    {
      label: 'Prepare for day 2',
      description: 'Move into production checklist items, backups, exposure, and observability.',
      docId: 'user-guide/openbaocluster/next-steps',
    },
  ]}
/>

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

Use one of these as your starting point, then adjust only the fields your environment actually requires.

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

## Before you move on

<Checklist
  title="Before you call the path production-ready"
  items={[
    'Select the Hardened profile unless this environment is strictly non-production.',
    'Use self-init unless you are intentionally carrying a manual bootstrap workflow.',
    'Keep TLS on External or ACME for hardened clusters.',
    'Leave admission policies enabled unless you are doing controlled break-glass recovery.',
    'Decide how backups will be configured and tested before the first production upgrade.',
    'Use RollingUpdate by default and switch to BlueGreen only when parallel validation is worth the complexity.',
  ]}
/>

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

<OutcomePanel
  title="You are ready to install once the path is boring to explain."
  tone="success"
  actions={[
    {label: 'Install the operator', docId: 'user-guide/operator/installation'},
    {label: 'See the production checklist', docId: 'user-guide/openbaocluster/operations/production-checklist'},
  ]}
>
  <p>The next step should feel mechanical, not exploratory. You should already know:</p>

  - whether you are running multi-tenant or single-tenant mode
  - which security profile, TLS mode, and bootstrap path are acceptable
  - whether Helm or raw manifests own the install
  - which readiness conditions matter before the environment is exposed to real users
</OutcomePanel>

## See Also

- [Operator Invariants](../../architecture/operator-invariants.md)
- [Production Checklist](../openbaocluster/operations/production-checklist.md)
- [Multi-Tenancy](../openbaotenant/multi-tenancy.md)
