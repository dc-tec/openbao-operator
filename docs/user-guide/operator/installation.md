---
title: Operator Installation
description: Install OpenBao Operator with the intended tenancy mode, rendered identity, and verification checks.
slug: /get-started/install
hide_title: true
pageType: task
journey: get-started
journeyStep: 2
---

<!-- id: installation-guide -->

<PageHeader
  title="Install the operator for the intended tenancy mode"
  lede="Choose a supported install path, keep the rendered namespace and identity explicit, and verify the controller wiring for the tenancy mode you intend to run."
/>

<Checklist
    title="Installation preflight"
    items={[
      'confirm Kubernetes compatibility and cluster-admin access for CRDs, RBAC, and admission policies',
      'decide whether Helm or raw manifests own the install lifecycle',
      'decide whether you are staying multi-tenant or intentionally switching to single-tenant mode',
      'if you stay multi-tenant, know who will create the first OpenBaoTenant and in which namespace',
      'pin a released operator version for production instead of relying on floating tags',
    ]}
  />


<JourneyRail
  title="Installation sequence"
  current={2}
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
      label: 'Onboard the target namespace',
      description: 'In the default multi-tenant path, let OpenBaoTenant introduce the namespace, then create the cluster.',
      docId: 'user-guide/openbaotenant/onboarding',
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

## Prerequisites

<Callout type="tip" title="Requirements">

- **Kubernetes**: v1.33+ (see [Compatibility](../../reference/compatibility.md))
- **kubectl**: Installed and configured
- **Permissions**: Cluster-admin access for CRDs, RBAC, and ValidatingAdmissionPolicies
- **Helm** (optional): v3.12+ for Helm-based installation

</Callout>

<Callout type="note" title="Deployment Modes">

The operator supports two deployment modes:

- **Multi-Tenant** (default): Platform teams providing OpenBao-as-a-Service
- **Single-Tenant**: Individual teams deploying OpenBao for their application

See [Single-Tenant Mode](single-tenant-mode.md) for single-tenant deployments.

</Callout>

## Install profiles

Use this table to choose the supported install path before changing values or overlays. For most environments, the default answer is Helm plus multi-tenant mode unless your namespace ownership model requires a different shape.

<DecisionTable
  title="Supported installation paths"
  columns={['Intent', 'Recommended path', 'Change these settings', 'Verify these outputs']}
  rows={[
    {
      cells: ['Default shared production install', 'Helm, multi-tenant mode', 'release namespace, chart version, controller/provisioner sizing', 'controller and provisioner pods in the rendered operator namespace'],
      emphasis: 'recommended',
    },
    {
      cells: ['Dedicated team namespace', 'Helm, tenancy.mode=single', 'tenancy.targetNamespace, optional release namespace', 'only the controller pod runs; WATCH_NAMESPACE matches the target namespace'],
    },
    {
      cells: ['Dedicated team namespace with custom Helm identity', 'Helm, tenancy.mode=single plus custom release name or fullnameOverride', 'release name or fullnameOverride, tenancy.targetNamespace, optional release namespace', 'rendered controller ServiceAccount name, single-tenant RoleBinding subject, admission-policy identity variables, JWT audience'],
    },
    {
      cells: ['Raw multi-tenant install with default identity', 'config/default', 'operator namespace only if you want to fork the default base', 'rendered namespace, controller and provisioner ServiceAccount names, admission policies'],
    },
    {
      cells: ['Raw multi-tenant install with custom identity', 'config/overlays/custom-identity', 'namespace, optional namePrefix', 'rendered ServiceAccount names, RoleBinding subjects, admission-policy identity variables, JWT audience'],
    },
    {
      cells: ['Raw single-tenant install', 'config/overlays/single-tenant', 'operator namespace in the overlay, target namespace in target_namespace_config.yaml', 'rendered operator namespace, WATCH_NAMESPACE, single-tenant RoleBinding subject'],
    },
    {
      cells: ['Raw single-tenant install with custom identity', 'config/overlays/single-tenant-custom-identity', 'namespace, optional namePrefix, target namespace in target_namespace_config.yaml', 'rendered operator namespace, controller ServiceAccount name, WATCH_NAMESPACE, single-tenant RoleBinding subject, admission-policy identity variables'],
    },
  ]}
/>

<Callout type="note" title="Single-tenant customization boundary">

Use `config/overlays/single-tenant` when you only need a custom operator namespace or target namespace.
Use `config/overlays/single-tenant-custom-identity` when you also need a custom operator identity, such as an extra `namePrefix`.

</Callout>

<Callout type="info" title="Default recommendation">

Start with Helm, keep the default multi-tenant mode, pin the chart release for production,
and leave admission policies enabled. Move away from that path only for explicit raw-manifest
control or single-tenant namespace ownership requirements.

</Callout>

## Installation

<Tabs groupId="helm-recommended-openshift-yaml-manifests-developer-source">

<TabItem value="helm-recommended" label="Helm (recommended)">

Install the operator using the official Helm chart. For production, pin the chart release explicitly with `--version`.

<Callout type="note" title="Rendered operator namespace">

The examples below use the default release namespace `openbao-operator-system`. If you install the chart into another namespace, replace it consistently in the commands and later verification steps.

</Callout>

<CommandBlock
  language="bash"
  label="apply"
  title="Install the Helm chart"
  code={`helm upgrade --install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \\
  --version <chart-version> \\
  --namespace openbao-operator-system \\
  --create-namespace`}
/>

### Common configuration

<CommandBlock
  language="bash"
  label="configure"
  title="Pin the chart release and right-size the controller"
  code={`helm upgrade --install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \\
  --version <chart-version> \\
  --namespace openbao-operator-system \\
  --create-namespace \\
  --set controller.replicas=2 \\
  --set controller.resources.limits.memory=512Mi`}
/>

1. Pin the chart release with `--version` for production deployments.
2. Run multiple replicas for high availability.
3. Adjust resource limits based on cluster size.

<Callout type="note" title="Chart release pinning vs image override">

For normal installs, pin the chart with `--version` and let the chart's `appVersion` select the matching operator image.
Use `image.tag` only when you intentionally need a non-default operator image for that chart, such as prerelease validation or a controlled override.

</Callout>

### Single-tenant with custom Helm identity

Helm already supports the equivalent of the raw-manifest custom-identity overlays through the release name and `fullnameOverride`.

<CommandBlock
  language="bash"
  label="configure"
  title="Install in single-tenant mode with a custom identity"
  code={`helm upgrade --install team-bao oci://ghcr.io/dc-tec/charts/openbao-operator \\
  --version <chart-version> \\
  --namespace platform-operators \\
  --create-namespace \\
  --set tenancy.mode=single \\
  --set tenancy.targetNamespace=openbao \\
  --set fullnameOverride=team-bao-operator`}
/>

Confirm with `helm template` or `helm get manifest` that:

1. the controller `ServiceAccount` name matches the rendered Helm fullname
2. the single-tenant `RoleBinding` subject points at that rendered controller `ServiceAccount`
3. admission-policy variables reference the same rendered operator namespace and controller `ServiceAccount` name
4. `OPENBAO_JWT_AUDIENCE` on the controller still matches the projected `openbao-token` audience

<Callout type="note">

The chart does not expose per-component custom `ServiceAccount` names. Use the release name or `fullnameOverride` to customize the operator identity while keeping the rendered RBAC and admission-policy references aligned.

</Callout>

### Artifact Hub

Discover package metadata and install snippets on Artifact Hub:

- [Search `openbao-operator` package](https://artifacthub.io/packages/search?repo=openbao-operator)

<Callout type="note">

Artifact Hub indexing can lag shortly after a release is published.

</Callout>

### Full values reference

| Parameter | Description | Default |
| :--- | :--- | :--- |
| `image.repository` | Operator image repository | `ghcr.io/dc-tec/openbao-operator` |
| `image.tag` | Image tag (defaults to appVersion) | `""` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `imagePullSecrets` | Registry credentials | `[]` |
| `platform` | Target platform (`auto`, `kubernetes`, `openshift`) | `auto` |
| `tenancy.mode` | `multi` or `single` | `multi` |
| `tenancy.targetNamespace` | Target namespace (single-tenant only) | `""` |
| `controller.replicas` | Controller replica count | `1` |
| `controller.resources` | Controller resource requests/limits | See values.yaml |
| `provisioner.replicas` | Provisioner replica count | `1` |
| `provisioner.resources` | Provisioner resource requests/limits | See values.yaml |
| `admissionPolicies.enabled` | Enable ValidatingAdmissionPolicies | `true` |
| `metrics.enabled` | Enable metrics endpoints | `true` |

[Full values.yaml](https://github.com/dc-tec/openbao-operator/blob/main/charts/openbao-operator/values.yaml)

<Callout type="info" title="Air-gapped environments">

To use private registries for the operator and its sidecars (init, backup, upgrade), see the [Air-Gapped / Private Registries](../openbaocluster/configuration/air-gapped.md) guide.

</Callout>

</TabItem>

<TabItem value="openshift" label="OpenShift">

For Red Hat OpenShift clusters, the operator defaults to platform auto-detection.
You can optionally force the platform mode to ensure compatibility with Security Context Constraints (SCC):

<CommandBlock
  language="bash"
  label="configure"
  title="Force OpenShift platform mode"
  code={`helm upgrade --install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \\
  --version <chart-version> \\
  --namespace openbao-operator-system \\
  --create-namespace \\
  --set platform=openshift`}
/>

<Callout type="tip" title="What this setting does">

This setting instructs the chart/operator to omit pinned `runAsUser` / `fsGroup` IDs in generated Pods, allowing OpenShift's SCC admission controller to inject namespace-scoped IDs automatically.

</Callout>

</TabItem>

<TabItem value="yaml-manifests" label="YAML Manifests">

Apply the installer manifest directly from a pinned GitHub Release:

<CommandBlock
  language="bash"
  label="apply"
  title="Apply the pinned installer manifest"
  code={`kubectl apply -f https://github.com/dc-tec/openbao-operator/releases/download/X.Y.Z/install.yaml`}
/>

<Callout type="note">

Replace `X.Y.Z` with the exact release you intend to run. Use `latest` only for throwaway evaluation, not for production installs.

</Callout>

Raw-manifest installs have three supported starting points:

- `config/default`: default multi-tenant install
- `config/overlays/custom-identity`: multi-tenant install with custom operator namespace or `namePrefix`
- `config/overlays/single-tenant`: direct single-tenant install without the provisioner
- `config/overlays/single-tenant-custom-identity`: direct single-tenant install without the provisioner plus custom operator identity support

<Callout type="tip" title="Custom namespace or prefix">

For raw-manifest installs with a custom operator namespace or extra name prefix, start from `config/overlays/custom-identity`. Set `namespace` there and optionally add `namePrefix`. The controller and provisioner ServiceAccount identities, RoleBinding subjects, and admission-policy identity checks follow the installed ServiceAccounts automatically.

</Callout>

<Callout type="tip" title="Single-tenant raw manifests">

For direct single-tenant installs, start from `config/overlays/single-tenant`. That overlay owns the operator namespace and target namespace wiring instead of relying on manual `WATCH_NAMESPACE` patches.

</Callout>

<Callout type="tip" title="Single-tenant with custom identity">

If you need single-tenant mode and a custom operator identity, such as an extra `namePrefix`, start from `config/overlays/single-tenant-custom-identity`. That overlay keeps the single-tenant namespace wiring and the controller admission-policy identity rewrites aligned in one supported path.

</Callout>

<Callout type="note" title="Operator JWT auth">

If you use custom raw-manifest identities together with manual OpenBao JWT configuration or self-init OIDC bootstrap, verify the rendered controller ServiceAccount name and namespace first. See [Operator Authentication](./operator-authentication#what-must-stay-aligned).

</Callout>

</TabItem>

<TabItem value="developer-source" label="Developer (Source)">

For local development and contribution:

<CommandBlock
  language="bash"
  label="apply"
  title="Install from source for development"
  code={`# Install CRDs
make install

# Deploy operator (uses Kustomize)
make deploy IMG=ghcr.io/dc-tec/openbao-operator:dev`}
/>

</TabItem>

</Tabs>

## Render verification

Use this checklist for raw-manifest installs.

### Multi-tenant with custom identity

Render the overlay:

<CommandBlock
  language="bash"
  label="inspect"
  title="Render the custom-identity overlay"
  code={`kubectl kustomize config/overlays/custom-identity`}
/>

Confirm:

1. the rendered operator namespace is the namespace you expect
2. the controller and provisioner `ServiceAccount` names match your intended install identity
3. `RoleBinding` and `ClusterRoleBinding` subjects point at those rendered ServiceAccounts
4. admission-policy variables reference the same rendered namespace and ServiceAccount names
5. `OPENBAO_JWT_AUDIENCE` on the controller matches the projected `openbao-token` audience

See [Operator Authentication](./operator-authentication#what-must-stay-aligned) for the OpenBao-side JWT binding checks.

### Single-tenant raw manifests

Render the overlay:

<CommandBlock
  language="bash"
  label="inspect"
  title="Render the single-tenant overlay"
  code={`kubectl kustomize config/overlays/single-tenant`}
/>

Confirm:

1. the rendered operator namespace matches `config/overlays/single-tenant/kustomization.yaml`
2. `WATCH_NAMESPACE` on the controller matches `config/overlays/single-tenant/target_namespace_config.yaml`
3. the single-tenant `RoleBinding` namespace matches the same target namespace
4. the controller `ServiceAccount` subject in that `RoleBinding` points at the rendered operator namespace

If you customize the single-tenant overlay beyond those supported fields, treat the render output as the source of truth.

### Single-tenant with custom identity

Render the overlay:

<CommandBlock
  language="bash"
  label="inspect"
  title="Render the single-tenant custom-identity overlay"
  code={`kubectl kustomize config/overlays/single-tenant-custom-identity`}
/>

Confirm:

1. the rendered operator namespace matches `config/overlays/single-tenant-custom-identity/kustomization.yaml`
2. the rendered controller `ServiceAccount` name matches the same overlay after any `namePrefix`
3. `WATCH_NAMESPACE` on the controller matches `config/overlays/single-tenant-custom-identity/target_namespace_config.yaml`
4. the single-tenant `RoleBinding` subject points at the rendered controller `ServiceAccount`
5. controller admission-policy variables reference the same rendered namespace and `ServiceAccount` name

## Verify installation

Check that the operator pods are running:

<CommandBlock
  language="bash"
  label="verify"
  title="Check that the operator pods are running"
  code={`kubectl get pods -n <operator-namespace>`}
/>

Expected output (multi-tenant mode):

<CommandBlock
  language="text"
  label="output"
  title="Expected output in multi-tenant mode"
  code={`NAME                                              READY   STATUS    RESTARTS   AGE
<controller-pod>                                  1/1     Running   0          1m
<provisioner-pod>                                 1/1     Running   0          1m`}
/>

<Callout type="success" title="Verify the operator namespace before continuing">

Use this install checkpoint to verify more than pods in `Running`:

- the controller and provisioner pods match the tenancy mode you chose
- the rendered namespace and ServiceAccount names match your install plan
- admission policies are installed when they are supposed to be
- in the default multi-tenant path, you know which namespace will receive the first `OpenBaoTenant`
- your next step is tenant onboarding or cluster creation, not more install debugging

</Callout>

## Upgrading

### Helm upgrades

<Callout type="warning" title="CRD updates">

Helm does not automatically upgrade CRDs. For releases with CRD changes:

1. Apply CRDs from the release assets first:
    ```bash
    kubectl apply -f https://github.com/dc-tec/openbao-operator/releases/download/X.Y.Z/crds.yaml
    ```
2. Then upgrade the Helm release:
    ```bash
    helm upgrade openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
      --version X.Y.Z \
      --namespace openbao-operator-system
    ```

</Callout>

### YAML manifest upgrades

```bash
kubectl apply -f https://github.com/dc-tec/openbao-operator/releases/download/X.Y.Z/install.yaml
```

For custom raw-manifest installs, use your rendered namespace, ServiceAccount names, and policy names rather than assuming the repository defaults.

## Uninstallation

<Tabs groupId="helm-yaml-manifests">

<TabItem value="helm" label="Helm">

```bash
helm uninstall openbao-operator --namespace openbao-operator-system
```

<Callout type="danger" title="CRDs Retained">

Helm does not delete CRDs by design. To fully remove:
```bash
kubectl delete crd openbaoclusters.openbao.org openbaorestores.openbao.org openbaotenants.openbao.org
```

</Callout>

</TabItem>

<TabItem value="yaml-manifests" label="YAML Manifests">

```bash
kubectl delete -f https://github.com/dc-tec/openbao-operator/releases/download/X.Y.Z/install.yaml
```

</TabItem>

</Tabs>

## Next steps

<NextActions
  items={[
    {
      label: 'Onboard the target namespace',
      description: 'In the default multi-tenant path, create OpenBaoTenant, then create the first cluster.',
      docId: 'user-guide/openbaotenant/onboarding',
    },
    {
      label: 'Create your first cluster',
      description: 'Move straight into the first cluster guide after onboarding is complete or when you intentionally chose single-tenant mode.',
      docId: 'user-guide/openbaocluster/getting-started',
    },
    {
      label: 'Review single-tenant mode',
      description: 'Use the namespace-scoped branch when one team directly owns one namespace and one cluster lifecycle.',
      docId: 'user-guide/operator/single-tenant-mode',
    },
  ]}
/>
