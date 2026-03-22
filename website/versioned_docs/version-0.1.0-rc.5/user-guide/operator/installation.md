---
slug: /get-started/install
---

# Operator Installation
<!-- id: installation-guide -->

This guide covers deploying the OpenBao Operator to your Kubernetes cluster.

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

## Install Profiles

Use this table to choose the supported install path before you start changing values or overlays.

| Intent | Recommended path | Change these settings | Verify these outputs |
| :--- | :--- | :--- | :--- |
| Default shared production install | Helm, multi-tenant mode | release namespace, image tag, controller/provisioner sizing | controller and provisioner pods in the rendered operator namespace |
| Dedicated team namespace | Helm, `tenancy.mode=single` | `tenancy.targetNamespace`, optional release namespace | only the controller pod runs; `WATCH_NAMESPACE` matches the target namespace |
| Dedicated team namespace with custom Helm identity | Helm, `tenancy.mode=single` plus custom release name or `fullnameOverride` | release name or `fullnameOverride`, `tenancy.targetNamespace`, optional release namespace | rendered controller `ServiceAccount` name, single-tenant `RoleBinding` subject, admission-policy identity variables, JWT audience |
| Raw multi-tenant install with default identity | `config/default` | operator namespace only if you want to fork the default base | rendered namespace, controller and provisioner ServiceAccount names, admission policies |
| Raw multi-tenant install with custom identity | `config/overlays/custom-identity` | `namespace`, optional `namePrefix` | rendered ServiceAccount names, RoleBinding subjects, admission-policy identity variables, JWT audience |
| Raw single-tenant install | `config/overlays/single-tenant` | operator namespace in the overlay, target namespace in `target_namespace_config.yaml` | rendered operator namespace, `WATCH_NAMESPACE`, single-tenant RoleBinding subject |
| Raw single-tenant install with custom identity | `config/overlays/single-tenant-custom-identity` | `namespace`, optional `namePrefix`, target namespace in `target_namespace_config.yaml` | rendered operator namespace, controller `ServiceAccount` name, `WATCH_NAMESPACE`, single-tenant `RoleBinding` subject, admission-policy identity variables |

<Callout type="note" title="Single-Tenant Customization Boundary">

Use `config/overlays/single-tenant` when you only need a custom operator namespace or target namespace.
Use `config/overlays/single-tenant-custom-identity` when you also need a custom operator identity, such as an extra `namePrefix`.

</Callout>

## Installation

<Tabs groupId="helm-recommended-openshift-yaml-manifests-developer-source">

<TabItem value="helm-recommended" label="Helm (Recommended)">

Install the operator using the official Helm chart:

<Callout type="note" title="Rendered operator namespace">

The examples below use the default release namespace `openbao-operator-system`. If you install the chart into another namespace, replace it consistently in the commands and later verification steps.

</Callout>

```bash
helm install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
  --namespace openbao-operator-system \
  --create-namespace
```

### Common Configuration

```bash
helm install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
  --namespace openbao-operator-system \
  --create-namespace \
  --set image.tag=1.0.0 \
  --set controller.replicas=2 \
  --set controller.resources.limits.memory=512Mi
```

1. Pin to a specific version for production deployments.
2. Run multiple replicas for high availability.
3. Adjust resource limits based on cluster size.

### Single-Tenant With Custom Helm Identity

Helm already supports the equivalent of the raw-manifest custom-identity overlays through the release name and `fullnameOverride`.

```bash
helm install team-bao oci://ghcr.io/dc-tec/charts/openbao-operator \
  --namespace platform-operators \
  --create-namespace \
  --set tenancy.mode=single \
  --set tenancy.targetNamespace=openbao \
  --set fullnameOverride=team-bao-operator
```

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

### Full Values Reference

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

<Callout type="info" title="Air-Gapped Environments">

To use private registries for the operator and its sidecars (init, backup, upgrade), see the [Air-Gapped / Private Registries](../openbaocluster/configuration/air-gapped.md) guide.

</Callout>

</TabItem>

<TabItem value="openshift" label="OpenShift">

For Red Hat OpenShift clusters, the operator defaults to platform auto-detection.
You can optionally force the platform mode to ensure compatibility with Security Context Constraints (SCC):

```bash
helm install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
  --namespace openbao-operator-system \
  --create-namespace \
  --set platform=openshift
```

<Callout type="tip" title="What this does">

This setting instructs the chart/operator to omit pinned `runAsUser` / `fsGroup` IDs in generated Pods, allowing OpenShift's SCC admission controller to inject namespace-scoped IDs automatically.

</Callout>

</TabItem>

<TabItem value="yaml-manifests" label="YAML Manifests">

Apply the installer manifest directly from the GitHub Release:

```bash
kubectl apply -f https://github.com/dc-tec/openbao-operator/releases/latest/download/install.yaml
```

<Callout type="note">

This installs CRDs, RBAC, ValidatingAdmissionPolicies, and the operator deployments in `openbao-operator-system`.

</Callout>

Raw-manifest installs have three supported starting points:

- `config/default`: default multi-tenant install
- `config/overlays/custom-identity`: multi-tenant install with custom operator namespace or `namePrefix`
- `config/overlays/single-tenant`: direct single-tenant install without the provisioner
- `config/overlays/single-tenant-custom-identity`: direct single-tenant install without the provisioner plus custom operator identity support

<Callout type="tip" title="Custom Namespace Or Prefix">

For raw-manifest installs with a custom operator namespace or extra name prefix, start from `config/overlays/custom-identity`. Set `namespace` there and optionally add `namePrefix`. The controller and provisioner ServiceAccount identities, RoleBinding subjects, and admission-policy identity checks follow the installed ServiceAccounts automatically.

</Callout>

<Callout type="tip" title="Single-Tenant Raw Manifests">

For direct single-tenant installs, start from `config/overlays/single-tenant`. That overlay owns the operator namespace and target namespace wiring instead of relying on manual `WATCH_NAMESPACE` patches.

</Callout>

<Callout type="tip" title="Single-Tenant With Custom Identity">

If you need single-tenant mode and a custom operator identity, such as an extra `namePrefix`, start from `config/overlays/single-tenant-custom-identity`. That overlay keeps the single-tenant namespace wiring and the controller admission-policy identity rewrites aligned in one supported path.

</Callout>

<Callout type="note" title="Operator JWT Auth">

If you use custom raw-manifest identities together with manual OpenBao JWT configuration or self-init OIDC bootstrap, verify the rendered controller ServiceAccount name and namespace first. See [Operator Authentication](authn.md#custom-install-checklist).

</Callout>

</TabItem>

<TabItem value="developer-source" label="Developer (Source)">

For local development and contribution:

```bash
# Install CRDs
make install

# Deploy operator (uses Kustomize)
make deploy IMG=ghcr.io/dc-tec/openbao-operator:dev
```

</TabItem>

</Tabs>

## Render Verification

Use this checklist for raw-manifest installs before you apply the manifests.

### Multi-Tenant With Custom Identity

Render the overlay:

```bash
kubectl kustomize config/overlays/custom-identity
```

Confirm:

1. the rendered operator namespace is the namespace you expect
2. the controller and provisioner `ServiceAccount` names match your intended install identity
3. `RoleBinding` and `ClusterRoleBinding` subjects point at those rendered ServiceAccounts
4. admission-policy variables reference the same rendered namespace and ServiceAccount names
5. `OPENBAO_JWT_AUDIENCE` on the controller matches the projected `openbao-token` audience

See [Operator Authentication](authn.md#custom-install-checklist) for the OpenBao-side JWT binding checks.

### Single-Tenant Raw Manifests

Render the overlay:

```bash
kubectl kustomize config/overlays/single-tenant
```

Confirm:

1. the rendered operator namespace matches `config/overlays/single-tenant/kustomization.yaml`
2. `WATCH_NAMESPACE` on the controller matches `config/overlays/single-tenant/target_namespace_config.yaml`
3. the single-tenant `RoleBinding` namespace matches the same target namespace
4. the controller `ServiceAccount` subject in that `RoleBinding` points at the rendered operator namespace

If you customize the single-tenant overlay beyond those supported fields, treat the render output as the source of truth.

### Single-Tenant With Custom Identity

Render the overlay:

```bash
kubectl kustomize config/overlays/single-tenant-custom-identity
```

Confirm:

1. the rendered operator namespace matches `config/overlays/single-tenant-custom-identity/kustomization.yaml`
2. the rendered controller `ServiceAccount` name matches the same overlay after any `namePrefix`
3. `WATCH_NAMESPACE` on the controller matches `config/overlays/single-tenant-custom-identity/target_namespace_config.yaml`
4. the single-tenant `RoleBinding` subject points at the rendered controller `ServiceAccount`
5. controller admission-policy variables reference the same rendered namespace and `ServiceAccount` name

## Verify Installation

Check that the operator pods are running:

    ```bash
    kubectl get pods -n <operator-namespace>
    ```

Expected output (multi-tenant mode):

```
NAME                                              READY   STATUS    RESTARTS   AGE
    <controller-pod>      1/1     Running   0          1m
    <provisioner-pod>     1/1     Running   0          1m
```

<Callout type="success" title="Ready">

Once both pods show `Running`, proceed to [Getting Started](../openbaocluster/getting-started.md) to deploy your first OpenBao cluster.

</Callout>

## Upgrading

### Helm Upgrades

<Callout type="warning" title="CRD Updates">

Helm does not automatically upgrade CRDs. For releases with CRD changes:

1. Apply CRDs from the release assets first:
    ```bash
    kubectl apply -f https://github.com/dc-tec/openbao-operator/releases/download/X.Y.Z/crds.yaml
    ```
2. Then upgrade the Helm release:
    ```bash
    helm upgrade openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
      --namespace openbao-operator-system
    ```

</Callout>

### YAML Manifest Upgrades

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
kubectl delete -f https://github.com/dc-tec/openbao-operator/releases/latest/download/install.yaml
```

</TabItem>

</Tabs>

## Next Steps

<div class="grid cards" markdown>

- **Deploy a Cluster**

    ---

    Create your first OpenBaoCluster.

    [Getting Started](../openbaocluster/getting-started.md)

- **Multi-Tenancy**

    ---

    Onboard teams with OpenBaoTenant.

    [Multi-Tenancy](../openbaotenant/overview.md)

- **Single-Tenant**

    ---

    Simplified deployment for single teams.

    [Single-Tenant Mode](single-tenant-mode.md)

</div>
