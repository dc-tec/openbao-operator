---
title: Compatibility Matrix
description: Compatibility matrix for OpenBao Operator covering CI validation scope, support posture, and production guidance.
pageType: reference
journey: reference
---

<PageHeader
  title="Compatibility matrix"
  lede="Validated versions, support posture, and production guidance for the current pre-GA line."
/>

<Callout type="note" title="Terminology">

Use these terms consistently on this page:

- **Validated**: explicitly exercised in CI.
- **Best-effort supported**: eligible for issue triage and fixes on the latest stable release line only.
- **Recommended for production**: stable releases using the documented `Hardened` profile, `selfInit`, admission enforcement, explicit version pinning, and staged upgrade validation.

</Callout>

## Pre-GA posture

The current stable release line is intended for real deployments, but it remains pre-GA. The served CRD API is `openbao.org/v1alpha1`, and minor releases may introduce breaking changes. See [Deprecation Policy](deprecation-policy.md) and [Support Policy](support-policy.md) for the full release contract.

<DecisionTable
  kind="reference"
  title="Kubernetes versions"
  columns={['Version', 'Validated', 'Support posture', 'Production note']}
  rows={[
    {
      cells: ['v1.35', 'PR gate, release gate, and nightly E2E', 'Best-effort supported on the latest stable line', 'Primary validated baseline'],
      emphasis: 'recommended',
    },
    {
      cells: ['v1.34', 'Release gate, nightly E2E, and performance baseline', 'Best-effort supported on the latest stable line', 'Minimum validated version'],
    },
    {
      cells: ['v1.36', 'Not validated for the current release line', 'Tracked as the next supported Kubernetes candidate', 'Adopt after controller-runtime, Kind, and release-gate coverage are available'],
      emphasis: 'caution',
    },
    {
      cells: ['v1.33', 'Not validated for the current release line', 'May work but is not release-gated for the current line', 'Validate in staging before carrying this version into the current pre-GA line'],
      emphasis: 'caution',
    },
    {
      cells: ['v1.32', 'Not validated', 'Out of support scope', 'Do not use for the current pre-GA line'],
      emphasis: 'caution',
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Platforms"
  columns={['Platform', 'Validated', 'Support posture', 'Production note']}
  rows={[
    {
      cells: ['Kubernetes', 'Default CI and nightly path', 'Best-effort supported on the latest stable line', 'Default recommended platform path'],
      emphasis: 'recommended',
    },
    {
      cells: ['OpenShift', 'Helm rendering regression checks, manifest admission tests, and focused platform E2E', 'Best-effort supported on the latest stable line', 'Validate on your target cluster before production rollout'],
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="OpenBao versions"
  columns={['Version', 'Validated', 'Support posture', 'Production note']}
  rows={[
    {
      cells: ['2.5.x', 'PR gate, nightly E2E, and config compatibility checks', 'Best-effort supported on the latest stable line', 'Primary validated target'],
      emphasis: 'recommended',
    },
    {
      cells: ['2.4.x', 'Nightly config compatibility checks', 'Best-effort supported on the latest stable line', 'Validate workload behavior and upgrade flow in staging before production rollout'],
    },
    {
      cells: ['2.3.x', 'Not validated', 'Deprecated and out of support scope', 'Upgrade before production use'],
      emphasis: 'caution',
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="CI validation matrix"
  caption="Versions and paths exercised by CI workflows."
  columns={['Workflow', 'Scope', 'Versions tested']}
  rows={[
    {
      cells: ['PR Gate', 'Logic, manifests, and primary compatibility path', 'K8s 1.35.1 + OpenBao 2.5.5'],
      emphasis: 'recommended',
    },
    {
      cells: ['Release Gate E2E', 'Stable release lifecycle coverage', 'K8s 1.34.3 and 1.35.1 + OpenBao 2.5.5'],
    },
    {
      cells: ['Nightly E2E', 'Full lifecycle coverage', 'K8s 1.34.3 and 1.35.1 + OpenBao 2.5.5'],
    },
    {
      cells: ['Nightly Config Compatibility', 'Render and config compatibility checks', 'OpenBao 2.4.4 and 2.5.5'],
    },
  ]}
/>

<Callout type="warning" title="Production upgrade rule">

Always validate new Kubernetes or OpenBao versions in a staging environment before upgrading production, even when they are listed as validated and best-effort supported.

</Callout>

<NextActions
  title="Related reference pages"
  items={[
    {
      label: 'Upgrade compatibility',
      description: 'Upgrade sequencing and rollback stance for operator upgrades.',
      docId: 'reference/operator-upgrade-compatibility',
    },
    {
      label: 'Support policy',
      description: 'Release-line maintenance beyond raw validation coverage.',
      docId: 'reference/support-policy',
    },
    {
      label: 'Known limitations',
      description: 'Current caveats and explicit non-goals for unsupported paths.',
      docId: 'reference/known-limitations',
    },
  ]}
/>
