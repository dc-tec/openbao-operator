---
title: Admission Policies
hide_title: true
pageType: concept
journey: security
description: How ValidatingAdmissionPolicy guardrails enforce managed-resource ownership, tenant onboarding boundaries, and fail-closed startup and runtime checks.
---

<PageHeader
  title="Admission policies and fail-closed behavior"
  lede="Kubernetes `ValidatingAdmissionPolicy` guardrails for key safety rules at the API boundary."
/>



<DiagramFrame
  title="Admission enforcement flow"
  caption="GitOps, human operators, and controller identities all cross the same API boundary. Admission guardrails stop invalid or dangerous objects before the reconcile loop has to repair them."
  code={`graph LR
    User["GitOps / human / controller write"] --> API["Kubernetes API"]
    API --> Policy["ValidatingAdmissionPolicy"]
    Policy -- "deny" --> Reject["Request rejected"]
    Policy -- "allow" --> Persist["Object persisted"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class User,API read;
    class Policy process;
    class Reject,Persist write;`}
/>

<DecisionTable
  title="Policy families"
  columns={['Family', 'What it protects', 'Representative policies']}
  rows={[
    {
      cells: [
        'Managed-resource ownership',
        'Prevents users, GitOps, and controllers from mutating operator-managed objects outside the allowed patterns.',
        '`openbao-lock-managed-resource-mutations`, controller StatefulSet self-protection, image digest enforcement.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Spec validation',
        'Rejects invalid `OpenBaoCluster`, `OpenBaoTenant`, and `OpenBaoRestore` objects before they persist.',
        '`openbao-validate-openbaocluster`, `openbao-validate-openbao-tenant`, `openbao-validate-openbaorestore`.',
      ],
    },
    {
      cells: [
        'Provisioner restrictions',
        'Constrains tenant onboarding, namespace mutation, and day-0 governance writes.',
        '`openbao-restrict-provisioner-rbac`, namespace-mutation, and tenant-governance policies.',
      ],
    },
    {
      cells: [
        'Controller restrictions',
        'Constrains controller RBAC, ServiceAccount creation, and Secret writes.',
        '`openbao-restrict-controller-rbac`, ServiceAccount, and Secret-write policies.',
      ],
    },
    {
      cells: [
        'Claim guardrails',
        'Protects materialized claim spec locking, claim-managed local cluster ownership, and service-catalog mutation boundaries.',
        '`openbao-lock-materialized-openbaoclusterclaim-spec`, `openbao-restrict-claim-managed-openbaocluster-mutations`, and `openbao-restrict-service-catalog-mutations`.',
      ],
    },
  ]}
/>

## Fail-closed startup and runtime behavior

<DecisionTable
  kind="reference"
  title="Admission dependency model"
  columns={['State', 'Operator behavior', 'Why']}
  rows={[
    {
      cells: [
        'Required policy set present and bound',
        'Startup and sensitive reconciliation proceed normally.',
        'The operator can rely on the API-level guardrails it expects.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Required policy set missing at startup',
        'Startup fails closed by default.',
        'It is safer to refuse operation than to reconcile privileged workflows without guardrails.',
      ],
    },
    {
      cells: [
        'Required policy disappears or becomes misbound later',
        'Sensitive reconciliation paths pause and surface degraded status.',
        'The admission dependency is part of the runtime safety model as well as startup validation.',
      ],
    },
    {
      cells: [
        'Unsafe mode explicitly enabled',
        'The operator can start without admission dependency enforcement.',
        'Reserved for development or explicit break-glass scenarios; materially weakens defense in depth.',
      ],
    },
  ]}
/>

The required fail-closed dependency set includes:

- `openbao-validate-openbaocluster`
- `openbao-validate-openbao-tenant`
- `openbao-validate-openbaorestore`
- `openbao-lock-controller-statefulset-mutations`
- `openbao-restrict-provisioner-rbac`
- `openbao-restrict-provisioner-namespace-mutations`
- `openbao-restrict-provisioner-tenant-governance`
- `openbao-restrict-controller-rbac`
- `openbao-restrict-controller-serviceaccounts`
- `openbao-restrict-controller-secret-writes`
- `openbao-lock-managed-resource-mutations`
- `openbao-enforce-managed-image-digests`

When the service-claim surface is enabled, the dependency set also includes:

- `openbao-lock-materialized-openbaoclusterclaim-spec`
- `openbao-restrict-claim-managed-openbaocluster-mutations`
- `openbao-restrict-service-catalog-mutations`

<Callout type="note" title="Claims add one mutating admission surface">

The claim path also uses a mutating admission webhook to resolve `spec.serviceOfferingRef` to the pinned immutable service-profile revision. The webhook is part of the supported claim install surface alongside the validating policies listed here.

</Callout>

<Callout type="warning" title="Unsafe mode is not a production posture">

Disabling admission policies is treated as unsafe mode. Even if the cluster otherwise uses Hardened settings, turning off API-level guardrails weakens the operator’s defense-in-depth model substantially.

</Callout>

## Provisioner guardrails

<DecisionTable
  kind="reference"
  title="Provisioner policy goals"
  columns={['Policy area', 'What it constrains', 'Why it matters']}
  rows={[
    {
      cells: [
        'RBAC writes',
        'Only fixed Role and RoleBinding names, subjects, and allowed verbs are permitted.',
        'This prevents the provisioner from becoming a generic RBAC minting identity.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Namespace mutation',
        'Provisioner namespace updates are limited to fixed Pod Security label enforcement and blocked in system namespaces. In external Pod Security label mode, provisioner namespace mutations are denied entirely.',
        'Tenant onboarding should not become a generic namespace-mutation channel, and platform-owned namespace labels should stay owned by platform policy.',
      ],
    },
    {
      cells: [
        'Tenant governance objects',
        'Only operator-owned `ResourceQuota` and `LimitRange` shapes are allowed for the fixed names.',
        'Day-0 guardrails should remain centrally shaped and not drift through arbitrary direct edits.',
      ],
    },
  ]}
/>

## Controller guardrails

<DecisionTable
  kind="reference"
  title="Controller policy goals"
  columns={['Policy area', 'What it constrains', 'Why it matters']}
  rows={[
    {
      cells: [
        'RBAC writes',
        'Only a narrow per-cluster pod-discovery and service-registration role pattern is allowed.',
        'This blocks RBAC self-escalation inside tenant namespaces.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'ServiceAccount writes',
        'Only operator-managed main, backup, restore, and upgrade ServiceAccounts are allowed.',
        'The controller should not become a general-purpose ServiceAccount management identity.',
      ],
    },
    {
      cells: [
        'Secret writes',
        'Only fixed operator-managed Secret names can be created or mutated.',
        'A broader RBAC grant should not silently become arbitrary tenant Secret mutation.',
      ],
    },
    {
      cells: [
        'Managed-resource mutation',
        'Drift on operator-managed StatefulSets, Services, Pods, and other objects is denied.',
        'This protects the reconciliation contract and keeps GitOps or manual edits from undermining the lifecycle model.',
      ],
    },
  ]}
/>

<Callout type="note" title="Admission canary">

The provisioner supports an optional admission canary that submits a dry-run RBAC request which must be denied. This adds evidence that policy enforcement is active, beyond the presence of the policy objects themselves.

</Callout>

## Configuration ownership

Admission policy is one of the reasons the operator can separate user intent from platform-owned configuration:

- user-owned surfaces stay in the CR where customization is supported
- operator-owned networking, seal, listener identity, and lifecycle wiring stay protected
- unsafe or drifted changes are rejected before they have to be repaired later

<NextActions
  title="Continue platform controls"
  items={[
    {
      label: 'RBAC architecture',
      description: 'How these policies reinforce the split-controller identity model.',
      docId: 'security/infrastructure/rbac',
    },
    {
      label: 'Network security',
      description: 'Default-deny network posture that complements admission enforcement.',
      docId: 'security/infrastructure/network-security',
    },
    {
      label: 'Threat model',
      description: 'STRIDE threats mitigated by the admission guardrails.',
      docId: 'security/fundamentals/threat-model',
    },
    {
      label: 'Open service claims',
      description: 'See the tenant-facing claim workflow that depends on the claim-specific mutating webhook and validating guardrails.',
      docId: 'user-guide/service-claims/overview',
    },
  ]}
/>
