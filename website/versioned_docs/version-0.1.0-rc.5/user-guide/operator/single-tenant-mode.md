---
title: Single-Tenant Mode
description: Use the controller-only install path when one team owns one namespace and does not need the default tenant-onboarding model.
slug: /get-started/single-tenant-mode
hide_title: true
pageType: task
journey: get-started
---

<PageHero
  eyebrow="Supporting decision"
  title="Use single-tenant mode when one team owns one namespace."
  lede="Single-tenant mode removes the Provisioner and lets the controller watch one target namespace directly. It is a good fit for dedicated team environments, but it is a branch from the default platform path rather than the starting point for every install."
  actions={[
    {label: 'Return to the decision guide', docId: 'user-guide/operator/deployment-decision-guide', variant: 'primary'},
    {label: 'Install the operator', docId: 'user-guide/operator/installation', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Use this page when you need to"
    items={[
      'run one operator for one team-owned namespace',
      'remove tenant onboarding and provisioner-driven namespace orchestration',
      'keep direct controller access scoped to a single target namespace',
      'verify how Helm or raw manifests render the target namespace and controller identity',
    ]}
  />
</PageHero>

<DecisionTable
  title="Stay on multi-tenant unless this is true"
  columns={['Question', 'Multi-tenant default', 'Choose single-tenant when', 'Go deeper']}
  rows={[
    {
      cells: [
        'Namespace ownership',
        'Platform teams or shared operators manage multiple target namespaces.',
        'One team directly owns one namespace and does not need the `OpenBaoTenant` onboarding flow.',
        'Tenant onboarding and RBAC',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Control-plane shape',
        'Controller plus Provisioner',
        'Controller only is the right operational model for this environment.',
        'Architecture overview',
      ],
    },
    {
      cells: [
        'Operational tradeoff',
        'More default platform structure in exchange for clearer shared-operator boundaries.',
        'A simpler dedicated setup matters more than cross-namespace platform workflows.',
        'Deployment decision guide',
      ],
    },
  ]}
/>

<DiagramFrame
  title="Single-tenant control path"
  caption="The controller watches one target namespace directly. There is no Provisioner and no tenant onboarding layer in front of the cluster resources."
  code={`graph LR
    subgraph OperatorNS["Operator namespace"]
      Controller["Controller"]
    end

    subgraph TargetNS["Target namespace"]
      Cluster["OpenBaoCluster"]
      STS["StatefulSet"]
      SVC["Services"]
      Secret["Workload-facing Secrets"]
    end

    Controller --> Cluster
    Cluster --> STS
    Cluster --> SVC
    Cluster --> Secret

    classDef control fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef data fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;

    class Controller control;
    class Cluster,STS,SVC,Secret data;`}
/>

## Install the single-tenant branch

<Tabs groupId="single-tenant-install-path">

<TabItem value="helm" label="Helm">

<CommandBlock
  language="bash"
  label="apply"
  title="Install the operator in single-tenant mode with Helm"
  code={`helm install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \\
  --namespace openbao-operator-system \\
  --create-namespace \\
  --set tenancy.mode=single \\
  --set tenancy.targetNamespace=openbao`}
>
  Replace `openbao-operator-system` and `openbao` with the namespaces you actually intend to keep operating. In this mode the target namespace is the controller watch scope, not just an example value.
</CommandBlock>

<DecisionTable
  kind="reference"
  title="Helm settings to care about"
  columns={['Setting', 'What it controls', 'Default or note']}
  rows={[
    {
      cells: ['`tenancy.mode`', 'Switches the operator to the controller-only path', '`single` for this branch'],
      emphasis: 'recommended',
    },
    {
      cells: ['`tenancy.targetNamespace`', 'Sets the watched namespace and target RoleBinding scope', 'Defaults to the release namespace when unset'],
    },
    {
      cells: ['`fullnameOverride` or release name', 'Changes the rendered controller identity', 'Recheck JWT auth and RoleBinding subjects after custom naming'],
    },
    {
      cells: ['`admissionPolicies.enabled`', 'Controls ValidatingAdmissionPolicies for the install', 'Keep enabled unless your platform does not support them'],
    },
  ]}
/>

</TabItem>

<TabItem value="manifests" label="Raw manifests">

<CommandBlock
  language="bash"
  label="apply"
  title="Apply the single-tenant overlay"
  code={`kubectl apply -k config/overlays/single-tenant`}
>
  Use `config/overlays/single-tenant-custom-identity` instead when you need both the single-tenant branch and a custom operator identity.
</CommandBlock>

<CommandBlock
  language="yaml"
  label="configure"
  title="Set the target namespace before you apply the overlay"
  code={`apiVersion: v1
kind: ConfigMap
metadata:
  name: single-tenant-settings
  annotations:
    config.kubernetes.io/local-config: "true"
data:
  WATCH_NAMESPACE: my-openbao`}
>
  The target namespace must match both the controller `WATCH_NAMESPACE` value and the generated RoleBinding namespace.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Render the overlay once before you apply it"
  code={`kubectl kustomize config/overlays/single-tenant`}
>
  Confirm that the controller ServiceAccount subject, RoleBinding namespace, and `WATCH_NAMESPACE` value all point at the same intended target.
</CommandBlock>

</TabItem>

</Tabs>

<Callout type="note" title="What changes in this branch">

Single-tenant mode removes the Provisioner, skips `OpenBaoTenant`, and gives the controller direct namespace-scoped access instead.
That is simpler for a dedicated team, but it also means the operator is no longer modeling the default shared-platform boundary for you.

</Callout>

## Verify the install before you create a cluster

<CommandBlock
  language="bash"
  label="inspect"
  title="Check that only the controller is running"
  code={`kubectl get pods -n <operator-namespace>`}
>
  In single-tenant mode you should see the controller deployment running, but not a Provisioner pod.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Confirm the watched namespace matches the intended target"
  code={`kubectl get deploy -n <operator-namespace> openbao-operator-controller \\
  -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="WATCH_NAMESPACE")].value}'`}
>
  This value should match the target namespace you plan to use for the first `OpenBaoCluster`.
</CommandBlock>

## Migration guidance

<DecisionTable
  title="When to migrate between tenancy modes"
  columns={['Move', 'Use it when', 'Main work before or after', 'Watch for']}
  rows={[
    {
      cells: [
        'Multi-tenant to single-tenant',
        'A dedicated team is taking full ownership of one namespace and no longer needs tenant onboarding.',
        'Re-render the operator, remove `OpenBaoTenant` usage, and verify the direct RoleBinding scope.',
        'Direct namespace permissions replace the previous tenant boundary.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Single-tenant to multi-tenant',
        'You need shared platform ownership or want to return to the default onboarding model.',
        'Re-enable the Provisioner and model namespace access through `OpenBaoTenant` resources.',
        'Manual single-tenant RoleBindings must not linger after the switch.',
      ],
    },
  ]}
/>

<NextActions
  title="Return to the main path"
  items={[
    {
      label: 'Install the operator',
      description: 'Go back to the install guide once the tenancy branch and rendered namespaces are clear.',
      docId: 'user-guide/operator/installation',
    },
    {
      label: 'Create your first cluster',
      description: 'Move into the first cluster guide after the controller watch scope and target namespace are verified.',
      docId: 'user-guide/openbaocluster/getting-started',
    },
    {
      label: 'Review operator identity and access',
      description: 'Double-check auth and RoleBinding surfaces when you are also customizing names or namespaces.',
      docId: 'user-guide/operator/identity-and-access',
    },
  ]}
/>
