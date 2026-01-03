# Operator Deployment
<!-- id: installation-guide -->

## Installation

!!! tip "Prerequisites"
    - **Kubernetes Cluster**: v1.33+ (see `docs/reference/compatibility.md`)
    - **kubectl**: Installed and configured
    - **Permissions**: Cluster-admin permissions to install CRDs and RBAC

    Default namespace: `openbao-operator-system`.

=== ":material-package: Helm (Recommended)"

    Install via the official Helm chart:

    ```sh
    kubectl create namespace openbao-operator-system

    helm install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
      --version <chart-version> \
      --namespace openbao-operator-system
    ```

    [:material-arrow-right: View on Artifact Hub](https://artifacthub.io/packages/helm/openbao-operator/openbao-operator)

=== ":material-file-document-multiple-outline: YAML Manifests"

    Download `install.yaml` from the GitHub Release and apply it directly:

    ```sh
    # Download from Release assets
    kubectl apply -f install.yaml
    ```

=== ":material-code-brackets: Developer (Source)"

    For local development and contribution:

    ```sh
    # Requires Kustomize
    make install
    make deploy IMG=ghcr.io/dc-tec/openbao-operator:dev
    ```

## Upgrading the operator

### Helm upgrades and CRDs

Helm does not reliably upgrade CRDs from the chart `crds/` directory on `helm upgrade`.
For releases that change CRDs:

1. Apply updated CRDs from the GitHub Release assets (or from `config/crd/bases/`) first.
2. Then run `helm upgrade`.

### Verifying the install

```sh
kubectl -n openbao-operator-system get pods
```
