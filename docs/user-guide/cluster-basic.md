# 4. Create a Basic OpenBaoCluster

[Back to User Guide index](README.md)

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
  tls:
    enabled: true
    rotationPeriod: "720h"
  storage:
    size: "10Gi"
  deletionPolicy: Retain
```

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
