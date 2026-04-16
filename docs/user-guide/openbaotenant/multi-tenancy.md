---
title: Multi-Tenant Security
description: Default shared-operator security model, including namespace introduction, split controllers, RBAC boundaries, and tenant guardrails.
slug: /tenant-onboarding/multi-tenant-security
hide_title: true
pageType: concept
journey: security
---

<PageHeader
  title="Shared-operator boundaries"
  lede="This page covers namespace introduction, split controllers, RBAC boundaries, and tenant guardrails in the default multi-tenant model."
/>



<Callout type="success" title="Default production model">

Multi-tenant mode is the recommended production operating model for OpenBao Operator.
Use [Single-Tenant Mode](../operator/single-tenant-mode.md) for dedicated environments where one team directly owns one namespace and does not need the default tenant-onboarding boundary.

</Callout>

<DiagramFrame
  title="Control-plane split"
  caption="Namespace introduction and tenant guardrails stay with the Provisioner. Workload reconciliation stays with the tenant-scoped Controller. That separation prevents one long-running identity from both granting and consuming tenant access."
  code={`graph LR
    subgraph Cluster["Shared Kubernetes cluster"]
      Admin["Platform admin or namespace owner"] --> Tenant["OpenBaoTenant"]
      Tenant --> Provisioner["Provisioner controller"]
      Provisioner --> Guardrails["Namespace RBAC and guardrails"]
      Guardrails --> Controller["Tenant-scoped controller access"]
      Controller --> ClusterObj["OpenBaoCluster and workload resources"]
      ClusterObj --> Jobs["Backup, restore, and upgrade jobs"]
    end

    classDef actor fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef control fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef data fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class Admin actor;
    class Tenant,Provisioner,Controller control;
    class Guardrails,ClusterObj,Jobs data;`}
/>

<DecisionTable
  title="Control-plane security boundaries"
  columns={['Surface', 'Primary owner', 'Why it stays separate', 'If you shortcut it']}
  rows={[
    {
      cells: [
        'Namespace introduction',
        'Provisioner',
        'The Provisioner can introduce tenant RBAC and guardrails without becoming the steady-state workload reconciler.',
        'You blur the boundary between granting namespace access and consuming it.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Workload reconciliation',
        'Controller',
        'The Controller stays namespace-scoped and only acts where onboarding already introduced access.',
        'A cluster-wide controller increases blast radius for config and Secret-facing mistakes.',
      ],
    },
    {
      cells: [
        'Tenant user access',
        'Tenant-scoped roles',
        'Users can manage clusters without automatically gaining Secret read access.',
        'Operators and tenant users start sharing too much of the same trust surface.',
      ],
    },
    {
      cells: [
        'Backup and restore execution',
        'Job-specific identities',
        'Day 2 jobs need narrower, time-bounded capabilities than the main controller.',
        'The controller role quietly becomes a universal credential for routine and destructive operations.',
      ],
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Default tenant roles"
  columns={['Role', 'Scope', 'Use it for', 'What it does not allow']}
  rows={[
    {
      cells: [
        '`openbaocluster-admin-role`',
        'Cluster',
        'Platform-level administration and exceptional cluster ownership.',
        'It should not be the normal tenant user path.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        '`openbaocluster-editor-role`',
        'Namespace',
        'Managing `OpenBaoCluster` resources in an onboarded tenant namespace.',
        'It does not grant broad Secret read access.',
      ],
    },
    {
      cells: [
        '`openbaotenant-editor-role`',
        'Namespace',
        'Self-service onboarding through `OpenBaoTenant` in the same namespace.',
        'It does not allow arbitrary cross-namespace onboarding.',
      ],
    },
  ]}
/>

<DecisionTable
  title="Isolation layers to validate"
  columns={['Layer', 'Default model', 'Validate this', 'Go deeper']}
  rows={[
    {
      cells: [
        'RBAC',
        'Namespace-scoped operator access introduced by `OpenBaoTenant`, with editor roles that avoid direct Secret reads.',
        'Check RoleBindings and `kubectl auth can-i` results for tenant users and operator identities.',
        'RBAC architecture',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Network policy',
        'OpenBao workloads default toward restricted ingress, and backup or restore jobs use their own network contract.',
        'Confirm cross-tenant traffic is blocked and backup egress is intentionally scoped.',
        'Network security',
      ],
    },
    {
      cells: [
        'Quota and namespace guardrails',
        'Onboarding applies default tenant quotas, limit ranges, and guardrail labels before the first cluster lands.',
        'Inspect the tenant namespace for the expected quota, limit range, and labels after onboarding.',
        'Onboard the target namespace',
      ],
    },
    {
      cells: [
        'Backup storage isolation',
        'Each tenant should use object-storage credentials or prefixes that do not overlap with other tenants.',
        'Make sure backup credentials cannot list or read other tenants\' snapshot paths.',
        'Backup operations',
      ],
    },
  ]}
/>

## Validate the shared-operator boundary

<CommandBlock
  language="bash"
  label="inspect"
  title="Inspect tenant guardrails after onboarding"
  code={`kubectl get openbaotenant <name> -n <namespace> -o yaml
kubectl get rolebinding,resourcequota,limitrange,networkpolicy -n <target-namespace>`}
>
  This gives you the fast check: the onboarding request is provisioned, the namespace-scoped RBAC exists, and the expected tenant guardrails were actually introduced.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Check tenant access stays scoped"
  code={`kubectl auth can-i create openbaoclusters.openbao.org -n <target-namespace> --as <tenant-user>
kubectl auth can-i get secrets -n <target-namespace> --as <tenant-user>`}
>
  The normal tenant editor path should allow cluster lifecycle work without granting broad Secret reads by default.
</CommandBlock>

<Callout type="note" title="Supplemental policy engines are optional">

Kyverno or Gatekeeper can still add cluster-wide guardrails, but they are not the primary tenant-isolation mechanism here.
The primary controls are the split-controller model, namespace introduction through `OpenBaoTenant`, built-in RBAC, and admission policy enforcement.

</Callout>

<NextActions
  title="Go deeper"
  items={[
    {
      label: 'Tenancy & governance',
      description: 'Return to the higher-level OpenBaoTenant model when you need the control-plane rationale rather than the security checklist.',
      docId: 'user-guide/openbaotenant/overview',
    },
    {
      label: 'Onboard the target namespace',
      description: 'Use the task page when you are actually creating the first tenant onboarding request.',
      docId: 'user-guide/openbaotenant/onboarding',
    },
    {
      label: 'RBAC architecture',
      description: 'Review the exact Kubernetes permission boundaries behind the Provisioner and Controller split.',
      docId: 'security/infrastructure/rbac',
    },
    {
      label: 'Network security',
      description: 'Go deeper on workload and job network policy assumptions in shared clusters.',
      docId: 'security/infrastructure/network-security',
    },
  ]}
/>
