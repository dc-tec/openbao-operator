# Operator Deployment
<!-- id: installation-guide -->

## Prerequisites

- **Kubernetes Cluster**: v1.33+ (see `docs/reference/compatibility.md`)
- **kubectl**: Installed and configured
- **Permissions**: Cluster-admin permissions to install CRDs and RBAC

Default namespace: `openbao-operator-system`.

## Install via Helm (recommended)

```sh
kubectl create namespace openbao-operator-system

helm install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
  --version <chart-version> \
  --namespace openbao-operator-system
```

## Install via release `install.yaml`

Download `install.yaml` from the GitHub Release and apply it:

```sh
kubectl apply -f install.yaml
```

## Developer install (Kustomize)

For local development only:

```sh
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
