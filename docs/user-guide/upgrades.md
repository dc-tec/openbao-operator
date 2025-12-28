# 12. Upgrades

[Back to User Guide index](README.md)

## 12.1 Upgrade Strategies

The OpenBao Operator supports two upgrade strategies:

| Strategy | Description | Use Case |
|----------|-------------|----------|
| **RollingUpdate** (default) | Updates pods one-by-one with leader step-down | Standard upgrades, minimal resource usage |
| **BlueGreen** | Creates a parallel "Green" cluster before switching traffic | Zero-downtime upgrades, safer rollback path |

Configure the strategy in your `OpenBaoCluster` spec:

```yaml
spec:
  updateStrategy:
    type: BlueGreen  # or RollingUpdate (default)
    blueGreen:
      autoPromote: true  # Automatically promote after sync (default: true)
      verification:
        minSyncDuration: "30s"  # Wait before promotion (optional)
```

---

## 12.2 Upgrade Authentication

Upgrade operations that require OpenBao permissions run in Kubernetes Jobs (upgrade executor). These Jobs authenticate to OpenBao via JWT using a projected ServiceAccount token.

The upgrade executor requires:

- `spec.upgrade.executorImage`
- `spec.upgrade.jwtAuthRole`

### 12.2.1 JWT Auth (Preferred)

Configure JWT Auth for upgrade operations:

1. **Create the JWT Auth role in OpenBao:**

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: upgrade-cluster
spec:
  selfInit:
    enabled: true
    requests:
      # Create upgrade policy
      - name: create-upgrade-policy
        operation: update
        path: sys/policies/acl/upgrade
        data:
          policy: |
            path "sys/health" {
              capabilities = ["read"]
            }
            path "sys/step-down" {
              capabilities = ["update"]
            }
            path "sys/storage/raft/snapshot" {
              capabilities = ["read"]
            }
      # Create JWT Auth role for upgrades
      - name: create-upgrade-jwt-role
        operation: update
        path: auth/jwt/role/upgrade
        data:
          role_type: jwt
          bound_audiences: ["openbao-internal"]
          bound_claims:
            kubernetes.io/namespace: openbao
            kubernetes.io/serviceaccount/name: upgrade-cluster-upgrade-serviceaccount
          token_policies: upgrade
          ttl: 1h
  upgrade:
    executorImage: openbao/upgrade-executor:v0.1.0
    jwtAuthRole: upgrade
    preUpgradeSnapshot: true
```

1. **Configure the upgrade manager to use the role:**

```yaml
spec:
  upgrade:
    executorImage: openbao/upgrade-executor:v0.1.0
    jwtAuthRole: upgrade
```

---

## 12.3 Upgrade ServiceAccount

The operator automatically creates `<cluster-name>-upgrade-serviceaccount` when upgrade authentication is configured. This ServiceAccount:

- Is used by upgrade executor Jobs for JWT Auth (via projected ServiceAccount token)
- Is mounted into the Job Pod via a projected token volume with audience `openbao-internal`
- Is owned by the OpenBaoCluster for automatic cleanup

---

## 12.4 Performing Upgrades

To upgrade an OpenBao cluster, update the `spec.version` field:

```sh
kubectl -n security patch openbaocluster dev-cluster \
  --type merge -p '{"spec":{"version":"2.5.0"}}'
```

### 12.4.1 Rolling Update Strategy

The upgrade manager will:

1. Validate the target version
2. Perform pre-upgrade validation (cluster health, quorum, leader detection)
3. Create a pre-upgrade snapshot (if `spec.upgrade.preUpgradeSnapshot` is enabled)
4. Perform a rolling upgrade, pod-by-pod, with leader step-down handling
5. Update `Status.CurrentVersion` when complete

### 12.4.2 Blue/Green Upgrade Strategy

The Blue/Green upgrade follows a 7-phase process:

```
Idle → DeployingGreen → JoiningMesh → Syncing → Promoting → DemotingBlue → Cleanup → Idle
```

| Phase | Description |
|-------|-------------|
| **Idle** | No upgrade in progress |
| **DeployingGreen** | Creating Green StatefulSet with new version, waiting for pods to be ready and unsealed |
| **JoiningMesh** | Green pods join the Raft cluster as non-voters |
| **Syncing** | Green pods replicate data from Blue leader |
| **Promoting** | Green pods promoted to voters |
| **DemotingBlue** | Blue pods demoted to non-voters, Green leader election verified |
| **Cleanup** | Blue pods removed from Raft, Blue StatefulSet deleted |

**Key benefits:**

- Zero-downtime: traffic only switches after Green is fully synced
- Safe rollback: Blue cluster remains available until cleanup phase
- Verification: automatic sync verification before promotion

---

## 12.5 Monitoring Upgrades

Monitor upgrade progress:

```sh
kubectl -n security get openbaocluster dev-cluster -o yaml
```

Check `Status.BlueGreen` for Blue/Green upgrade progress:

```yaml
status:
  blueGreen:
    phase: Syncing
    blueRevision: "e8983038c519c718"
    greenRevision: "269c4f7ba494030d"
    startTime: "2024-01-15T10:30:00Z"
```

Check `Status.Conditions` for upgrade state:

```sh
kubectl -n security get openbaocluster dev-cluster -o jsonpath='{.status.conditions}' | jq
```
