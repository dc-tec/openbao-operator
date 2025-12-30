# Basic Cluster Creation

## Prerequisites

- **OpenBao Operator**: Installed and running (see [Installation](installation.md))
- **Storage Class**: Default storage class configured in the cluster

## Configuration

The minimal spec focuses on version, image, replicas, TLS, and storage:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: dev-cluster
  namespace: security
spec:
  version: "2.1.0"
  image: "openbao/openbao:2.1.0"
  replicas: 3
  profile: Development
  tls:
    enabled: true
    rotationPeriod: "720h"
  storage:
    size: "10Gi"
  deletionPolicy: Retain
```

## Operation

Apply the resource:

```sh
kubectl apply -f dev-cluster.yaml
```

Check the CR status:

```sh
kubectl -n security get openbaoclusters.openbao.org dev-cluster -o yaml
```

Look for:

- `status.phase`
- `status.readyReplicas`
- `status.initialized` (becomes `true` after the cluster has been initialized)
- `status.conditions` (especially `Available`, `TLSReady`, `Degraded`, `EtcdEncryptionWarning`)
