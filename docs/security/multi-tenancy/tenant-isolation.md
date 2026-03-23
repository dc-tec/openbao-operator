---
title: Tenant Isolation
hide_title: true
pageType: concept
journey: security
description: How namespace introduction, split controller identities, admission policy, and network boundaries combine into the operator's tenant-isolation model.
---

<PageHero
  variant="compact"
  eyebrow="Security / Tenant Isolation"
  title="Make tenant access explicit instead of discoverable."
  lede="The operator's multi-tenant model depends on deliberate namespace introduction. A tenant namespace becomes manageable only after onboarding introduces the controller through fixed RBAC, applies namespace guardrails, and keeps the identity that grants access separate from the identity that consumes it."
  actions={[
    {label: "Open RBAC architecture", docId: "security/infrastructure/rbac", variant: "primary"},
    {label: "Review tenant onboarding", docId: "user-guide/openbaotenant/onboarding", variant: "secondary"},
  ]}
>
  <Checklist
    title="Use this page when you need to"
    items={[
      "understand what the shared-service model actually guarantees between namespaces",
      "review the difference between self-service and centrally managed onboarding",
      "connect tenant onboarding back to RBAC, admission, and network controls",
      "decide whether the operator's isolation model fits your platform trust assumptions",
    ]}
  />
</PageHero>

<DecisionTable
  title="Isolation pillars"
  columns={["Pillar", "What it protects", "Primary mechanism"]}
  rows={[
    {
      cells: [
        "Namespace introduction",
        "The controller does not become present in a namespace by accident.",
        "`OpenBaoTenant` onboarding creates the fixed Role and RoleBinding that introduce the controller deliberately.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Identity separation",
        "Provisioning and workload management do not share one broad credential.",
        "The provisioner grants access, while the controller uses tenant-scoped access without minting it.",
      ],
    },
    {
      cells: [
        "Admission guardrails",
        "Unsafe RBAC writes and operator-object drift are blocked at the API boundary.",
        "Validating admission policies constrain names, subjects, and managed-object mutation patterns.",
      ],
    },
    {
      cells: [
        "Network and namespace hardening",
        "Cross-tenant traffic and insecure sidecar drift are reduced by default.",
        "Default-deny `NetworkPolicy` and Restricted Pod Security labels apply at onboarding time.",
      ],
    },
  ]}
/>

<DiagramFrame
  title="Tenant introduction flow"
  caption="The provisioner writes the access grant into the target namespace, but the ongoing controller uses that access without being able to mint or broaden it freely."
  code={`flowchart TD
    Request["OpenBaoTenant request"] --> Provisioner["Provisioner"]
    Provisioner --> Namespace["Target namespace"]
    Provisioner --> Role["Tenant Role"]
    Provisioner --> Binding["Tenant RoleBinding"]
    Provisioner --> Guardrails["Namespace guardrails"]
    Binding --> Controller["Controller ServiceAccount"]
    Controller --> Workload["OpenBao workload"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Request,Namespace read;
    class Provisioner,Controller process;
    class Role,Binding,Guardrails,Workload write;`}
/>

## Onboarding models

<DecisionTable
  title="Choose the governance model"
  columns={["Model", "Who creates the request", "Primary constraint", "Why you would use it"]}
  rows={[
    {
      cells: [
        "Self-service",
        "A namespace admin creates `OpenBaoTenant` inside the same namespace.",
        "`spec.targetNamespace` must match `metadata.namespace`.",
        "Use this when teams manage their own namespaces and you want the least central coordination without granting cross-namespace privilege.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Centralized onboarding",
        "A platform-admin workflow creates the request in the operator namespace.",
        "Normal tenant users must not be able to write onboarding requests there.",
        "Use this when quotas, guardrails, or namespace vending are controlled centrally.",
      ],
    },
  ]}
/>

<Callout type="note" title="Task versus model">

This page explains the isolation contract. Use <SiteLink docId="user-guide/openbaotenant/onboarding">Onboard the target namespace</SiteLink> when you need the concrete onboarding workflow and field-level configuration.

</Callout>

## What the model guarantees

<DecisionTable
  kind="reference"
  title="Operational guarantees"
  columns={["Guarantee", "What it means", "Primary control"]}
  rows={[
    {
      cells: [
        "No namespace discovery as a normal workflow",
        "The controller does not need to list namespaces to find tenants.",
        "Namespaces are introduced through onboarding rather than through global discovery.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "No cross-tenant Secret browsing",
        "The controller should not treat tenant Secrets as generic cluster inventory.",
        "Secret access is name-scoped, role-scoped, and guarded by admission policy.",
      ],
    },
    {
      cells: [
        "No all-powerful long-running operator credential",
        "The component that grants access does not also reconcile every tenant workload with the same credential.",
        "Provisioner/controller identity split.",
      ],
    },
    {
      cells: [
        "Namespace-level runtime baseline",
        "Tenant namespaces start from Restricted pod-security enforcement and default network isolation.",
        "Provisioner-owned namespace labels and network-policy defaults.",
      ],
    },
  ]}
/>

## Assumptions and residual risk

<DecisionTable
  kind="reference"
  title="Assumptions you still need to own"
  columns={["Assumption", "Why it matters", "What to do"]}
  rows={[
    {
      cells: [
        "Admission stays enabled",
        "Without it, the RBAC and managed-object model loses an important API-level backstop.",
        "Treat unsafe mode as a deliberate exception and not as the normal multi-tenant posture.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "The surrounding cluster is trustworthy",
        "Node compromise, cluster-admin access, or hostile CNI behavior are outside what namespace isolation can solve.",
        "Pair the operator model with normal cluster hardening, audit, and node-security controls.",
      ],
    },
    {
      cells: [
        "Shared external systems are partitioned appropriately",
        "Object storage, PKI, and KMS systems can still become cross-tenant blast-radius points if configured broadly.",
        "Separate identities, prefixes, or trust roots per tenant or per environment where required.",
      ],
    },
  ]}
/>

<NextActions
  title="Continue tenant isolation"
  items={[
    {
      label: "RBAC architecture",
      description: "See the identity split that makes namespace introduction safe in the first place.",
      docId: "security/infrastructure/rbac",
    },
    {
      label: "Network security",
      description: "Review the default-deny network posture that complements the RBAC boundary.",
      docId: "security/infrastructure/network-security",
    },
    {
      label: "Tenant onboarding",
      description: "Switch to the user-guide workflow when you need to create or review an `OpenBaoTenant` request.",
      docId: "user-guide/openbaotenant/onboarding",
    },
  ]}
/>
