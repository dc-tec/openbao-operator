---
description: Step-by-step recipe for the validated local cross-cluster DR lane on k3d with shared Transit auto-unseal, RustFS snapshots, and Gateway API TLS passthrough.
---

# k3d Cross-Cluster DR Bootstrap

This recipe boots the validated local cross-cluster DR proving ground with:

- one infra k3d cluster hosting shared trust services
- one source k3d cluster
- one target k3d cluster
- shared Transit auto-unseal
- shared RustFS object storage
- Gateway API TLS passthrough on all three clusters

!!! success "Validated manually"
    This recipe matches the local DR lane proven end to end on March 16, 2026 in the project validation environment, including source backup, restore into the target cluster, target unseal, and source-state verification on the target endpoint.

## Prerequisites

- Docker, `k3d`, `kubectl`, `make`, and `jq` are installed.
- You can pull the public signed `edge` operator and helper images.

!!! tip "Validated defaults"
    The validated lane uses the public signed `edge` lane by default:

    - `DR_OPERATOR_IMAGE=ghcr.io/dc-tec/openbao-operator:edge`
    - `DR_OPERATOR_VERSION=edge`

## Inputs

The validated local lane uses these fixed names and endpoints:

| Value | Default | Purpose |
| :--- | :--- | :--- |
| Infra context | `k3d-openbao-dr-infra` | Shared Transit provider cluster |
| Source context | `k3d-openbao-dr-source` | Primary cluster |
| Target context | `k3d-openbao-dr-target` | Recovery target cluster |
| Source hostname | `bao-dr-source.example.com` | Source OpenBao ingress hostname |
| Target hostname | `bao-dr-target.example.com` | Target OpenBao ingress hostname |
| Transit endpoint | `https://host.k3d.internal` | Shared trust-services endpoint |
| Snapshot bucket | `openbao-dr-backups` | Shared RustFS bucket |
| Transit key | `openbao-dr-shared-unseal` | Shared DR seal key |

!!! note "Host bindings are implementation-specific"
    Local host port mappings are not part of the validated DR contract. The validated architecture depends on distinct source, target, and shared Transit endpoints, but the exact local port bindings are specific to your k3d environment.

## Step 1: Bootstrap the DR environment

Bootstrap your local DR environment with the cluster automation you use for k3d. One validated implementation grouped the cluster, gateway, trust-services, and operator setup into a single bootstrap target.

```bash
k3d cluster create ...
```

The validated bootstrap target performs the following:

- starts the shared RustFS instance and creates the bucket
- creates the infra, source, and target k3d clusters
- installs the Gateway API experimental bundle in each cluster
- installs a dedicated Traefik passthrough edge in each cluster
- bootstraps a shared external OpenBao trust-services endpoint in the infra cluster
- syncs the shared Transit token and CA bundle Secret into the source and target namespaces
- installs one single-tenant OpenBao Operator instance into the source and target clusters

## Step 2: Apply the source and target clusters

Apply the source and target `OpenBaoCluster` manifests that match the architecture invariants from [k3d Cross-Cluster DR with Shared Transit and RustFS](../../architectures/local/k3d-cross-cluster-dr-transit-rustfs.md).

```bash
kubectl --context <source-context> apply -f source-openbaocluster.yaml
kubectl --context <target-context> apply -f target-openbaocluster.yaml
```

This applies:

- the source `OpenBaoCluster` in `openbaocluster-dr-source`
- the target `OpenBaoCluster` in `openbaocluster-dr-target`
- the target-side S3-compatible endpoint wiring used for the restore path

## Operations

### Verify source and target readiness

Check the source cluster:

```bash
kubectl --context k3d-openbao-dr-source -n openbaocluster-dr-source \
  get openbaocluster openbaocluster-dr-source \
  -o jsonpath='{.status.phase}{"\n"}{.status.readyReplicas}{"\n"}{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

Check the target cluster:

```bash
kubectl --context k3d-openbao-dr-target -n openbaocluster-dr-target \
  get openbaocluster openbaocluster-dr-target \
  -o jsonpath='{.status.phase}{"\n"}{.status.readyReplicas}{"\n"}{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

The important steady-state expectations are:

- `phase=Running`
- `readyReplicas=1`
- `Available=True`
- `OpenBaoInitialized=True`
- `OpenBaoSealed=False`

### Verify the source and target passthrough endpoints

Check the source health endpoint:

```bash
curl -ksS --resolve bao-dr-source.example.com:10443:127.0.0.1 \
  https://bao-dr-source.example.com:10443/v1/sys/health
```

Check the target health endpoint:

```bash
curl -ksS --resolve bao-dr-target.example.com:11443:127.0.0.1 \
  https://bao-dr-target.example.com:11443/v1/sys/health
```

### Verify the pre-restore bootstrap state

The validated source cluster starts with `source-demo-password` and a `dr-control` marker of `phase1-source`:

```bash
SOURCE_TOKEN="$(
  curl -ksS --resolve bao-dr-source.example.com:10443:127.0.0.1 \
    -H 'Content-Type: application/json' \
    -d '{"password":"source-demo-password"}' \
    https://bao-dr-source.example.com:10443/v1/auth/userpass/login/demo-admin \
  | jq -r '.auth.client_token'
)"

curl -ksS --resolve bao-dr-source.example.com:10443:127.0.0.1 \
  -H "X-Vault-Token: ${SOURCE_TOKEN}" \
  https://bao-dr-source.example.com:10443/v1/secret/data/dr-control
```

The validated target cluster starts with `target-demo-password` and a `dr-control` marker of `phase1-target`:

```bash
TARGET_TOKEN="$(
  curl -ksS --resolve bao-dr-target.example.com:11443:127.0.0.1 \
    -H 'Content-Type: application/json' \
    -d '{"password":"target-demo-password"}' \
    https://bao-dr-target.example.com:11443/v1/auth/userpass/login/demo-admin \
  | jq -r '.auth.client_token'
)"

curl -ksS --resolve bao-dr-target.example.com:11443:127.0.0.1 \
  -H "X-Vault-Token: ${TARGET_TOKEN}" \
  https://bao-dr-target.example.com:11443/v1/secret/data/dr-control
```

## Next step

After bootstrap and pre-restore verification, follow [Cross-Cluster DR Restore with RustFS](../../runbooks/cross-cluster-dr-restore-rustfs.md).

## Related architecture

Use [k3d Cross-Cluster DR with Shared Transit and RustFS](../../architectures/local/k3d-cross-cluster-dr-transit-rustfs.md) for the topology, invariants, and validation scope behind this recipe.
