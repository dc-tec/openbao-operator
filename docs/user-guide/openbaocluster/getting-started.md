# Basic Cluster Creation

This guide walks you through creating your first OpenBaoCluster. Choose the path that matches your use case.

## Prerequisites

- **OpenBao Operator**: Installed and running (see [Installation](../operator/installation.md))
- **Tenancy**: In multi-tenant mode, the target namespace must be onboarded via `OpenBaoTenant` (see [Tenant Onboarding](../openbaotenant/onboarding.md)).
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
      version: "2.4.4"
      # image: "openbao/openbao:2.4.4" # Optional: inferred from version
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
      version: "2.4.4"
      # image: "openbao/openbao:2.4.4" # Optional: inferred from version
      replicas: 3
      profile: Hardened
      tls:
        enabled: true
        mode: External
      storage:
        size: "50Gi"
      selfInit:
        enabled: true
        oidc:
          enabled: true  # (1)!
        requests:
          # Configure user authentication FIRST to prevent lockout
          - name: enable-userpass
            operation: update
            path: sys/auth/userpass
            authMethod:
              type: userpass
              description: "Userpass authentication"
          - name: create-admin-policy
            operation: update
            path: sys/policies/acl/admin
            policy:
              policy: |
                path "*" {
                  capabilities = ["create", "read", "update", "delete", "list", "sudo"]
                }
          # Then configure secret engines
          - name: enable-kv-v2
            operation: update
            path: sys/mounts/secret
            secretEngine:
              type: kv
              description: "General purpose KV store"
              options:
                version: "2"
      unseal:
        type: awskms
        awskms:
          region: us-east-1
          kmsKeyID: alias/openbao-unseal
    ```

    1.  **OIDC bootstrap** enables the Operator to authenticate via JWT for cluster lifecycle operations (backups, upgrades). This is separate from user authentication.
    
    !!! danger "Lockout Prevention Required"
        **CRITICAL**: The `requests` array **must** include user authentication configuration (e.g., userpass, JWT, Kubernetes auth) BEFORE enabling self-init. OIDC bootstrap only provides Operator authentication, not user access. Enabling `selfInit.enabled: true` without user authentication in requests results in **permanent lockout** with no recovery options. See [Self-Initialization](configuration/self-init.md) for details.

    !!! tip "Production Checklist"
        Before deploying to production, complete the [Production Checklist](operations/production-checklist.md)
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

- [External Access](configuration/external-access.md) — Expose your cluster
- [Security Profiles](configuration/security-profiles.md) — Understand profile differences
- [Backups](operations/backups.md) — Configure disaster recovery
