# Air-Gapped & Private Registries

The OpenBao Operator supports deployment in air-gapped environments and usage of private container registries. This guide covers how to configure the operator and your clusters to use custom image repositories.

## Overview

There are two categories of images to consider:

1. **Operator Images**: The controller and provisioner images.
2. **Workload Images**: The OpenBao image and helper sidecars (backup, upgrade, config-init) injected by the operator.

## 1. Configuring the Operator

When installing the operator via Helm, you can update the operator's own image and provide global overrides for the sidecars it injects.

### Operator Image

Set the `image.repository` value in Helm:

```bash
helm install openbao-operator oci://... \
  --set image.repository=my-registry.corp/openbao-operator
```

### Sidecar Images (Global Defaults)

The operator injects helper containers for tasks like configuration (init), backups, and upgrades. By default, these pull from `ghcr.io/dc-tec`.

To override these globally for all clusters, set the following environment variables.

=== "values.yaml"

    ```yaml
    controller:
      extraEnv:
        - name: RELATED_IMAGE_OPENBAO
          value: "my-registry.corp/openbao/openbao"             # (1)!
        - name: OPERATOR_INIT_IMAGE_REPOSITORY
          value: "my-registry.corp/openbao-operator/config-init"
        - name: OPERATOR_BACKUP_IMAGE_REPOSITORY
          value: "my-registry.corp/openbao-operator/backup"
    ```

    1. Default OpenBao image used if `spec.image` is empty.

=== "Helm Command"

    ```bash
    helm install openbao-operator oci://... \
      --set "controller.extraEnv[0].name=RELATED_IMAGE_OPENBAO" \
      --set "controller.extraEnv[0].value=my-registry.corp/openbao/openbao"
    ```

| Environment Variable | Description | Default |
| :--- | :--- | :--- |
| `RELATED_IMAGE_OPENBAO` | Default OpenBao image if `spec.image` is empty | `docker.io/openbao/openbao` |
| `OPERATOR_INIT_IMAGE_REPOSITORY` | Repository for config-init container | `ghcr.io/dc-tec/openbao-config-init` |
| `OPERATOR_BACKUP_IMAGE_REPOSITORY` | Repository for backup executor | `ghcr.io/dc-tec/openbao-backup` |
| `OPERATOR_UPGRADE_IMAGE_REPOSITORY` | Repository for upgrade executor | `ghcr.io/dc-tec/openbao-upgrade` |

## 2. Configuring OpenBao Clusters

You can also override images and credentials at the `OpenBaoCluster` level.

### Specifying Images

You can specify the exact image for OpenBao and its sidecars in the CRD. This takes precedence over global defaults.

!!! note "Defaulting Logic"
    If `spec.image` is omitted, the operator infers it from `spec.version`. For example, version `2.4.4` defaults to `docker.io/openbao/openbao:2.4.4` (or the value of `RELATED_IMAGE_OPENBAO` env var). You only need to set `spec.image` if you are using a custom registry or a different tag.

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
spec:
  # Main OpenBao Image
  image: "my-registry.corp/openbao/openbao:2.0.0"

  # Config Init Image
  initContainer:
    image: "my-registry.corp/openbao-operator/config-init:v0.4.0"

  # Backup Sidecar
  backup:
    executorImage: "my-registry.corp/openbao-operator/backup:v0.4.0"
  
  # Upgrade Sidecar
  upgrade:
    executorImage: "my-registry.corp/openbao-operator/upgrade:v0.4.0"
```

### Image Pull Secrets

If your registry requires authentication, create a Kubernetes Secret and reference it in `spec.imagePullSecrets`.

1. **Create the Secret:**

    ```bash
    kubectl create secret docker-registry my-registry-creds \
      --docker-server=my-registry.corp \
      --docker-username=user \
      --docker-password=password
    ```

2. **Reference in Cluster:**

    ```yaml
    apiVersion: openbao.org/v1alpha1
    kind: OpenBaoCluster
    spec:
      imagePullSecrets:
        - name: my-registry-creds
      ...
    ```

The operator will attach these pull secrets to the StatefulSet, allowing Kubernetes to pull the images.

## Summary Checklist for Air-Gap

- [ ] **Mirror Images**: Retag and push the following images to your private registry:
  - `openbao-operator`
  - `openbao`
  - `openbao-config-init`
  - `openbao-backup`
  - `openbao-upgrade`
- [ ] **Install Operator**: Use Helm with `controller.extraEnv` to point to your mirrored sidecar repos.
- [ ] **Configure Clusters**: Use `spec.imagePullSecrets` if your registry is authenticated.
