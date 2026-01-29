# Operator Installation
<!-- id: installation-guide -->

This guide covers deploying the OpenBao Operator to your Kubernetes cluster.

## Prerequisites

!!! tip "Requirements"
    - **Kubernetes**: v1.33+ (see [Compatibility](../../reference/compatibility.md))
    - **kubectl**: Installed and configured
    - **Permissions**: Cluster-admin access for CRDs, RBAC, and ValidatingAdmissionPolicies
    - **Helm** (optional): v3.12+ for Helm-based installation

!!! note "Deployment Modes"
    The operator supports two deployment modes:

    - **Multi-Tenant** (default): Platform teams providing OpenBao-as-a-Service
    - **Single-Tenant**: Individual teams deploying OpenBao for their application

    See [Single-Tenant Mode](single-tenant-mode.md) for single-tenant deployments.

## Installation

=== ":material-package: Helm (Recommended)"

    Install the operator using the official Helm chart:

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

    [:material-arrow-right: Full values.yaml](https://github.com/dc-tec/openbao-operator/blob/main/charts/openbao-operator/values.yaml)

    !!! info "Air-Gapped Environments"
        To use private registries for the operator and its sidecars (init, backup, upgrade), see the [Air-Gapped / Private Registries](../openbaocluster/configuration/air-gapped.md) guide.

=== ":material-redhat: OpenShift"

    For Red Hat OpenShift clusters, the operator defaults to platform auto-detection.
    You can optionally force the platform mode to ensure compatibility with Security Context Constraints (SCC):

    ```bash
    helm install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
      --namespace openbao-operator-system \
      --create-namespace \
      --set platform=openshift
    ```

    !!! tip "What this does"
        This setting instructs the chart/operator to omit pinned `runAsUser` / `fsGroup` IDs in generated Pods, allowing OpenShift's SCC admission controller to inject namespace-scoped IDs automatically.

=== ":material-file-document-multiple-outline: YAML Manifests"

    Apply the installer manifest directly from the GitHub Release:

    ```bash
    kubectl apply -f https://github.com/dc-tec/openbao-operator/releases/latest/download/install.yaml
    ```

    !!! note
        This installs CRDs, RBAC, ValidatingAdmissionPolicies, and the operator deployments in `openbao-operator-system`.

=== ":material-code-brackets: Developer (Source)"

    For local development and contribution:

    ```bash
    # Install CRDs
    make install

    # Deploy operator (uses Kustomize)
    make deploy IMG=ghcr.io/dc-tec/openbao-operator:dev
    ```

## Verify Installation

Check that the operator pods are running:

```bash
kubectl get pods -n openbao-operator-system
```

Expected output (multi-tenant mode):

```
NAME                                              READY   STATUS    RESTARTS   AGE
openbao-operator-controller-xxxxxxxxxx-xxxxx      1/1     Running   0          1m
openbao-operator-provisioner-xxxxxxxxxx-xxxxx     1/1     Running   0          1m
```

!!! success "Ready"
    Once both pods show `Running`, proceed to [Getting Started](../openbaocluster/getting-started.md) to deploy your first OpenBao cluster.

## Upgrading

### Helm Upgrades

!!! warning "CRD Updates"
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

### YAML Manifest Upgrades

```bash
kubectl apply -f https://github.com/dc-tec/openbao-operator/releases/download/X.Y.Z/install.yaml
```

## Uninstallation

=== ":material-package: Helm"

    ```bash
    helm uninstall openbao-operator --namespace openbao-operator-system
    ```

    !!! danger "CRDs Retained"
        Helm does not delete CRDs by design. To fully remove:
        ```bash
        kubectl delete crd openbaoclusters.openbao.org openbaorestores.openbao.org openbaotenants.openbao.org
        ```

=== ":material-file-document-multiple-outline: YAML Manifests"

    ```bash
    kubectl delete -f https://github.com/dc-tec/openbao-operator/releases/latest/download/install.yaml
    ```

## Next Steps

<div class="grid cards" markdown>

- :material-rocket-launch: **Deploy a Cluster**

    ---

    Create your first OpenBaoCluster.

    [:material-arrow-right: Getting Started](../openbaocluster/getting-started.md)

- :material-account-group: **Multi-Tenancy**

    ---

    Onboard teams with OpenBaoTenant.

    [:material-arrow-right: Multi-Tenancy](../openbaotenant/overview.md)

- :material-target: **Single-Tenant**

    ---

    Simplified deployment for single teams.

    [:material-arrow-right: Single-Tenant Mode](single-tenant-mode.md)

</div>
