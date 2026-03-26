---
title: Threat Model
hide_title: true
pageType: concept
journey: security
description: Threat actors, trust boundaries, protected assets, and accepted residual risks for the OpenBao Operator control plane.
---

<PageHeader
  title="Start from the trust boundaries the operator is designed to defend."
  lede="This threat model focuses on the operator control plane, tenant isolation boundaries, and lifecycle workflows such as onboarding, upgrade, backup, and restore. It does not replace OpenBao's own internal threat model; it explains the extra surface introduced by running OpenBao through the operator."
/>

<Callout type="note" title="Scope">

This page models threats to the OpenBao Operator control plane, tenant isolation boundaries, and lifecycle workflows. It assumes the operator manages clusters it created and reconciles; generic import of arbitrary unmanaged OpenBao clusters is out of scope.

</Callout>

<DecisionTable
  title="Threat-model scope"
  columns={['Area', 'In scope', 'Why']}
  rows={[
    {
      cells: [
        'Operator control plane',
        'Yes',
        'Long-running operator identities, admission dependencies, and controller boundaries define most of the extra risk introduced by the operator.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Tenant isolation',
        'Yes',
        'Namespace introduction, RBAC boundaries, and managed-resource ownership are core to the multi-tenant model.',
      ],
    },
    {
      cells: [
        'Lifecycle workflows',
        'Yes',
        'Backup, restore, upgrade, and tenant onboarding introduce destructive or privileged execution paths.',
      ],
    },
    {
      cells: [
        'OpenBao internals unrelated to the operator',
        'No',
        'Those belong to OpenBao’s own product threat model rather than the operator-specific surface.',
      ],
    },
  ]}
/>

## Threat actors

<DecisionTable
  kind="reference"
  title="Actors that matter"
  columns={['Actor', 'Typical access', 'Why they matter']}
  rows={[
    {
      cells: [
        'Tenant author',
        'Namespace-scoped write access',
        'Can attempt to steer the operator, exploit weak isolation, or target Secrets and workload identities inside an onboarded namespace.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'GitOps pipeline or human cluster operator',
        'Namespace or cluster write access',
        'Can intentionally or accidentally mutate control-plane configuration, admission dependencies, or operator-managed resources.',
      ],
    },
    {
      cells: [
        'Compromised controller or provisioner Pod',
        'Operator-managed identity',
        'Tests whether the split-controller model and mutation locks actually narrow blast radius.',
      ],
    },
    {
      cells: [
        'Compromised OpenBao Pod or lifecycle Job',
        'Namespace workload identity',
        'Exercises the boundary between the steady-state service, generated Jobs, and the control plane.',
      ],
    },
    {
      cells: [
        'Compromised or misconfigured external dependency',
        'Object storage, PKI, KMS, ingress, or identity system',
        'The operator relies on those systems for unseal, TLS, backup, restore, and external access contracts.',
      ],
    },
  ]}
/>

## Trust boundaries

<DiagramFrame
  title="Trust zones"
  caption="Admission policy is the API enforcement boundary between submitted intent and persisted state. Operator identities, tenant workloads, and external systems each carry different trust assumptions."
  code={`graph TD
    subgraph Client_Zone ["Mutation clients"]
        GitOps["GitOps / human operator"]
        Tenant["Tenant author"]
    end

    subgraph Operator_Zone ["Operator identities"]
        Prov["Provisioner"]
        Ctrl["Controller"]
    end

    subgraph API_Zone ["Kubernetes API"]
        K8sAPI["Kubernetes API"]
        VAP["Admission policies"]
    end

    subgraph Tenant_Zone ["Tenant namespace"]
        Bao["OpenBao Pods"]
        Jobs["Backup / restore / upgrade Jobs"]
        Managed["Managed resources"]
    end

    subgraph External_Zone ["External systems"]
        Edge["Gateway / ingress"]
        Storage["Object storage"]
        Trust["Seal / PKI / identity systems"]
    end

    GitOps --> K8sAPI
    Tenant --> K8sAPI
    K8sAPI --> VAP
    Prov --> K8sAPI
    Ctrl --> K8sAPI
    Ctrl --> Bao
    Ctrl --> Jobs
    Ctrl --> Managed
    Edge --> Bao
    Bao --> Trust
    Jobs --> Storage

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class GitOps,Tenant,K8sAPI,Storage,Trust read;
    class Prov,Ctrl,VAP process;
    class Bao,Jobs,Managed,Edge write;`}
/>

## Protected assets

<DecisionTable
  kind="reference"
  title="Always-relevant assets"
  columns={['Asset', 'Risk', 'Why it matters']}
  rows={[
    {
      cells: [
        'Admission policies and bindings',
        'Critical',
        'They enforce the operator’s API-level safety model before unsafe intent persists.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Provisioner and controller identities',
        'Critical',
        'They define namespace onboarding and lifecycle authority across the cluster.',
      ],
    },
    {
      cells: [
        'Tenant RBAC and Secret allowlists',
        'High',
        'They control what an onboarded namespace can read, mutate, or discover.',
      ],
    },
    {
      cells: [
        'Raft data and snapshots',
        'High',
        'They contain the OpenBao state the operator is trying to protect and recover safely.',
      ],
    },
    {
      cells: [
        'Operator-managed configuration and generated job identities',
        'Medium',
        'They steer runtime behavior and disruptive workflows even when they do not directly store application secrets.',
      ],
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Conditional assets"
  columns={['Asset', 'Present when', 'Why it matters']}
  rows={[
    {
      cells: [
        'Root token Secret',
        'Bootstrap mode persists the initial root token',
        'This is a critical administrative credential and is intentionally avoided by Hardened self-init.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Static unseal keys',
        'Static unseal is used',
        'The root of trust sits in a Kubernetes Secret instead of an external trust system.',
      ],
    },
    {
      cells: [
        'Operator-managed CA key',
        '`OperatorManaged` TLS mode is selected',
        'This puts certificate authority material inside the cluster and is one reason that mode is not the Hardened path.',
      ],
    },
    {
      cells: [
        'Transit, cloud KMS, or HSM credentials',
        'External unseal or PKI integration is used',
        'These credentials and identity paths sit at a high-value trust boundary outside the workload itself.',
      ],
    },
  ]}
/>

## STRIDE analysis

<ExpandableCallout type="failure" title="Spoofing">

**Threats**

- A client or GitOps render path spoofs operator identity by drifting `ServiceAccount` names, policy subjects, or `RoleBinding` subjects.
- A workload or edge path spoofs cluster identity at the TLS boundary.
- Backup, restore, or upgrade Jobs inherit the wrong identity path.

<Callout type="success" title="Primary mitigations">

- Split provisioner and controller identities.
- Validate rendered install identities for Helm and raw-manifest overlays.
- Validate TLS SANs and trust sources before the cluster becomes ready.
- Use separate job `ServiceAccount` objects, identity checks, and Job-specific network controls.

</Callout>

</ExpandableCallout>

<ExpandableCallout type="warning" title="Tampering">

**Threats**

- A user, GitOps controller, or compromised namespace actor directly mutates operator-managed resources.
- A compromised provisioner broadens tenant RBAC or tenant guardrails.
- A tenant or operator steers backup or restore jobs toward unintended endpoints or credentials.

<Callout type="success" title="Primary mitigations">

- Lock operator-managed resources with admission policy.
- Restrict controller writes for RBAC, `ServiceAccount`, and Secret objects.
- Restrict provisioner namespace mutation and tenant-governance writes.
- Keep backup and restore credentials name-scoped and separately validated.

</Callout>

<Callout type="note" title="PVC posture">

Operator-managed PVCs are intentionally CR-driven and status-observed rather than fully admission-locked because Kubernetes storage controllers and CSI components also mutate PVCs during normal lifecycle.

</Callout>

</ExpandableCallout>

<ExpandableCallout type="note" title="Repudiation">

**Threats**

- High-value control-plane actions cannot be attributed later.
- Break-glass changes happen without a clear audit boundary.

<Callout type="success" title="Primary mitigations">

- Emit structured operator audit logs for startup gating, upgrades, backups, restore, and operation-lock transitions.
- Use Kubernetes API audit logs and admission denials as the primary mutation trail.
- Keep maintenance mode explicit and break-glass groups narrow by default.

</Callout>

</ExpandableCallout>

<ExpandableCallout type="danger" title="Information Disclosure">

**Threats**

- Secrets, credentials, or keys are exposed through logs or broad namespace access.
- TLS handling leaks sensitive material.
- Backup and restore credentials leak across workloads.

<Callout type="success" title="Primary mitigations">

- Never log secrets.
- Keep Secret access name-scoped without normal Secret enumeration.
- Use separate writer and reader roles for operator-managed Secrets.
- Keep ACME private keys in the OpenBao process instead of Kubernetes Secrets.
- Use separate workload identities for backup and restore Jobs.

</Callout>

</ExpandableCallout>

<ExpandableCallout type="failure" title="Denial of Service">

**Threats**

- Misconfiguration or tampering causes reconcile churn or blocks convergence.
- Day-2 operations collide or force unsafe concurrent mutations.
- Voluntary disruption or PDB tampering breaks quorum.
- Required admission policies disappear after startup.

<Callout type="success" title="Primary mitigations">

- Use controller rate limiting and bounded concurrency.
- Validate objects at admission before invalid state persists.
- Use explicit readiness conditions for API-server, Gateway, ACME, backup, and restore assumptions.
- Use a shared operation lock for disruptive workflows.
- Manage `PodDisruptionBudget` objects and lock them against drift.
- Re-check admission dependencies at runtime and pause privileged reconciliation when they disappear.

</Callout>

</ExpandableCallout>

<ExpandableCallout type="danger" title="Elevation of Privilege">

**Threats**

- A compromised controller broadens tenant privileges or writes to unrelated Secrets or `ServiceAccount` objects.
- A compromised provisioner mints broader tenant access or mutates protected namespaces.
- Unsafe mode or break-glass use weakens the API-level defense-in-depth boundary.

<Callout type="success" title="Primary mitigations">

- Split long-running identities and restrict controller writes.
- Keep Secret access name-scoped and allowlisted.
- Restrict provisioner RBAC, namespace mutation, and tenant-governance writes.
- Keep unsafe mode explicitly non-production and break-glass scoped.

</Callout>

</ExpandableCallout>

## Accepted residual risks

<Callout type="warning" title="Accepted posture">

- PVCs are intentionally soft-governed rather than fully admission-locked because Kubernetes and CSI controllers also update them.
- `UserAccessBootstrap` is best-effort signaling. The operator does not prove that arbitrary self-init requests create a usable human authentication path.
- Cloud KMS and external identity integrations are surfaced through conditions and validation, but still depend on systems outside the operator trust boundary.
- `unsafe mode` intentionally weakens the API-level safety model and is not a supported Hardened production posture.

</Callout>

<NextActions
  title="Continue the security model"
  items={[
    {
      label: 'Production posture',
      description: 'Move from threats into the supported security contract for Development versus Hardened.',
      docId: 'security/fundamentals/profiles',
    },
    {
      label: 'Secrets and trust material',
      description: 'Review how root tokens, unseal keys, and workload bootstrap identities behave across the lifecycle.',
      docId: 'security/fundamentals/secrets-management',
    },
    {
      label: 'Admission policies',
      description: 'See one of the main enforcement layers that turns this model into actual API guardrails.',
      docId: 'security/infrastructure/admission-policies',
    },
  ]}
/>
