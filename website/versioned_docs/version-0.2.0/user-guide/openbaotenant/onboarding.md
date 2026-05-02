---
title: Onboard the Target Namespace
description: Introduce the target namespace through OpenBaoTenant for the first cluster in the default multi-tenant path.
slug: /get-started/onboard-target-namespace
hide_title: true
pageType: task
journey: get-started
journeyStep: 3
---

<PageHeader
  title="Onboard the target namespace"
  lede="In the default multi-tenant model, you onboard a namespace with `OpenBaoTenant` before creating the first cluster there. This task installs the RBAC and tenant guardrails the operator depends on."
/>

<Callout type="note" title="Skip this in single-tenant mode">

If you intentionally chose [Single-Tenant Mode](../operator/single-tenant-mode.md), the controller watches one namespace directly and you do not use `OpenBaoTenant` for the first cluster path.

</Callout>

<Callout type="tip" title="OpenBaoTenant introduces a namespace; it does not create one">

Create the Kubernetes namespace through your normal platform workflow first, then apply `OpenBaoTenant` so the operator can install the namespace-scoped guardrails it depends on.

</Callout>

<Callout type="note" title="GitOps can submit tenant and cluster together">

If your GitOps pipeline applies `OpenBaoTenant` and `OpenBaoCluster` in the same sync, the cluster controllers pause cleanly until tenant onboarding is finished. The handoff is complete once the Provisioner has written the tenant `RoleBinding` and `OpenBaoTenant` reports `status.provisioned: true`.

</Callout>

<JourneyRail
  title="The first five moves"
  current={3}
  items={[
    {
      label: 'Choose a deployment model',
      description: 'Set tenancy, security posture, install method, and the main exceptions.',
      docId: 'user-guide/operator/deployment-decision-guide',
    },
    {
      label: 'Install the operator',
      description: 'Render the right namespace, identity, and admission posture.',
      docId: 'user-guide/operator/installation',
    },
    {
      label: 'Onboard the target namespace',
      description: 'Use OpenBaoTenant to introduce the namespace and let the operator create its default governance boundary.',
      docId: 'user-guide/openbaotenant/onboarding',
    },
    {
      label: 'Create your first cluster',
      description: 'Start with the closest cluster baseline and verify the important readiness signals.',
      docId: 'user-guide/openbaocluster/getting-started',
    },
    {
      label: 'Prepare for day 2',
      description: 'Move immediately into backups, access, upgrades, and production hardening.',
      docId: 'user-guide/openbaocluster/next-steps',
    },
  ]}
/>

<DecisionTable
  title="Choose the onboarding model"
  columns={['Model', 'Who creates OpenBaoTenant', 'Use it when', 'Watch for']}
  rows={[
    {
      cells: [
        'Self-service',
        'The namespace owner creates the `OpenBaoTenant` in the same target namespace.',
        'Teams already control their own namespaces and you want the default low-friction onboarding path.',
        '`metadata.namespace` and `spec.targetNamespace` must match.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Centrally managed',
        'A platform admin creates the `OpenBaoTenant` from the operator namespace.',
        'You want a stricter approval path or need custom quota and limit-range values for a namespace.',
        'Use the rendered operator namespace, not a guessed default.',
      ],
    },
  ]}
/>

<DiagramFrame
  title="What onboarding introduces"
  caption="OpenBaoTenant is the explicit namespace introduction point. The Provisioner reacts to that request and installs the namespace-scoped RBAC and default guardrails the operator depends on in the multi-tenant model."
  code={`graph LR
    Request["OpenBaoTenant"] --> Provisioner["Provisioner controller"]
    Provisioner --> RBAC["Namespace Role and RoleBinding"]
    Provisioner --> Quota["ResourceQuota and LimitRange"]
    Provisioner --> Labels["Tenant guardrail labels"]
    RBAC --> Cluster["OpenBaoCluster can now be managed in the namespace"]

    classDef request fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef control fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef data fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class Request request;
    class Provisioner control;
    class RBAC,Quota,Labels,Cluster data;`}
/>

## Apply the onboarding request

<Tabs groupId="tenant-onboarding-model">

<TabItem value="self-service" label="Self-service">

<CommandBlock
  language="yaml"
  label="configure"
  title="Create OpenBaoTenant in the target namespace"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoTenant
metadata:
  name: team-a-onboarding
  namespace: team-a-prod
spec:
  targetNamespace: team-a-prod`}
>
  In the self-service path, `metadata.namespace` and `spec.targetNamespace` must match. Self-service onboarding uses the default tenant guardrails and does not allow custom `quota` or `limitRange` values.
</CommandBlock>

</TabItem>

<TabItem value="centralized" label="Centrally managed">

<CommandBlock
  language="yaml"
  label="configure"
  title="Create OpenBaoTenant from the operator namespace"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoTenant
metadata:
  name: team-b-authorization
  namespace: <rendered-operator-namespace>
spec:
  targetNamespace: team-b-prod
  # Optional centrally managed guardrails:
  # quota:
  # limitRange:`}
>
  Use the rendered operator namespace from your install, not a hard-coded assumption, when platform admins create onboarding requests on behalf of teams.
</CommandBlock>

</TabItem>

</Tabs>

<CommandBlock
  language="bash"
  label="apply"
  title="Apply the onboarding request"
  code={`kubectl apply -f tenant-onboarding.yaml`}
/>

<Callout type="warning" title="Cross-namespace self-service is blocked">

If a namespace owner creates `OpenBaoTenant` in one namespace and targets a different namespace, the controller rejects it with a security violation instead of silently broadening access.

</Callout>

## Verify onboarding

<CommandBlock
  language="bash"
  label="inspect"
  title="Inspect the OpenBaoTenant status"
  code={`kubectl get openbaotenant <name> -n <namespace> -o yaml`}
>
  Confirm `status.provisioned: true` and a healthy `Provisioned` condition. In multi-tenant mode, that is the signal that the namespace-scoped RBAC handoff is complete and cluster reconciliation can proceed.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Verify the tenant RBAC handoff exists"
  code={`kubectl get rolebinding openbao-operator-tenant-rolebinding -n <target-namespace>`}
>
  This is the concrete handoff marker the controller waits for before it starts mutating `OpenBaoCluster` resources in that namespace.
</CommandBlock>

<DecisionTable
  kind="reference"
  title="Typical onboarding failures"
  columns={['Symptom', 'Most likely cause', 'Check first']}
  rows={[
    {
      cells: [
        'Provisioning is rejected with a security violation',
        'A self-service request targeted a namespace different from `metadata.namespace`',
        'The requested namespace pair and the onboarding model in use',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Provisioning never completes or clusters keep requeueing without creating workload resources',
        'The Provisioner is missing, unhealthy, or cannot write the tenant guardrails and tenant RoleBinding',
        'Operator install health, the Provisioner deployment, and the tenant `RoleBinding` in the target namespace',
      ],
    },
    {
      cells: [
        'Custom quotas are ignored',
        'The request came from the self-service path, which uses default guardrails only',
        'Whether the `OpenBaoTenant` was created from the operator namespace',
      ],
    },
  ]}
/>

<NextActions
  title="Continue the main path"
  items={[
    {
      label: 'Create your first cluster',
      description: 'Apply the first OpenBaoCluster only after the target namespace is provisioned and ready.',
      docId: 'user-guide/openbaocluster/getting-started',
    },
    {
      label: 'Review tenancy & governance',
      description: 'Use the concept page when you need the mental model behind OpenBaoTenant rather than just the task steps.',
      docId: 'user-guide/openbaotenant/overview',
    },
    {
      label: 'Review multi-tenant security',
      description: 'Go deeper on namespace isolation, RBAC, network policy, and guardrail assumptions in the shared-operator model.',
      docId: 'user-guide/openbaotenant/multi-tenancy',
    },
  ]}
/>
