# Basic Cluster Creation

This guide walks you through creating your first OpenBaoCluster. Choose the path that matches your use case.

## Prerequisites

- **OpenBao Operator**: Installed and running (see [Installation](installation.md))
- **Storage Class**: Default storage class configured in the cluster

## Choose Your Path

=== "Development (Local Testing)"

    For local development and testing. **Not suitable for production.**

    ```yaml
    apiVersion: openbao.org/v1alpha1
    kind: OpenBaoCluster
    metadata:
      name: dev-cluster
      namespace: default
    spec:
      version: "2.1.0"
      image: "openbao/openbao:2.1.0"
      replicas: 3
      profile: Development
      tls:
        enabled: true
        mode: OperatorManaged
        rotationPeriod: "720h"
      storage:
        size: "10Gi"
    ```

    !!! warning "Development Profile"
        The `Development` profile uses static auto-unseal and stores sensitive 
        material in Kubernetes Secrets. This is convenient for testing but 
        **insecure for production use**.

=== "Production"

    For production deployments with hardened security.

    ```yaml
    apiVersion: openbao.org/v1alpha1
    kind: OpenBaoCluster
    metadata:
      name: prod-cluster
      namespace: openbao
    spec:
      version: "2.1.0"
      image: "openbao/openbao:2.1.0"
      replicas: 3
      profile: Hardened
      tls:
        enabled: true
        mode: OperatorManaged
        rotationPeriod: "720h"
      storage:
        size: "50Gi"
      selfInit:
        enabled: true
        config:
          policies:
            - name: admin
              rules: |
                path "sys/*" { capabilities = ["create", "read", "update", "delete", "list", "sudo"] }
          authMethods:
            - type: kubernetes
              path: kubernetes
              config:
                kubernetes_host: "https://kubernetes.default.svc"
    ```

    !!! tip "Production Checklist"
        Before deploying to production, complete the [Production Checklist](production-checklist.md) 
        to ensure proper security configuration.

## Apply the Configuration

```sh
kubectl apply -f cluster.yaml
```

## Verify Deployment

Check the cluster status:

```sh
kubectl get openbaocluster <name> -n <namespace>
```

Watch pods come up:

```sh
kubectl get pods -l openbao.org/cluster=<name> -n <namespace> -w
```

## Check Status Conditions

```sh
kubectl describe openbaocluster <name> -n <namespace>
```

Look for:

- `status.phase` — Current lifecycle phase
- `status.readyReplicas` — Number of ready replicas
- `status.initialized` — `true` after cluster initialization
- `status.conditions`:
  - `Available` — Cluster is serving requests
  - `TLSReady` — TLS certificates are valid
  - `ProductionReady` — Security requirements met (Hardened only)
  - `Degraded` — Issues detected

## Next Steps

- [External Access](external-access.md) — Expose your cluster
- [Security Profiles](security-profiles.md) — Understand profile differences
- [Backups](backups.md) — Configure disaster recovery
