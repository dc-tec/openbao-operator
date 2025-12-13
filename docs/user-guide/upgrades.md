# 12. Upgrades

[Back to User Guide index](README.md)

### 12.1 Upgrade Authentication

The upgrade manager requires explicit authentication configuration via `spec.upgrade.jwtAuthRole` or `spec.upgrade.tokenSecretRef`. There is no fallback to backup authentication.

#### 12.1.1 JWT Auth (Preferred)

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
      # The ServiceAccount name is automatically <cluster-name>-upgrade-serviceaccount
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
    jwtAuthRole: upgrade  # Reference to the role created above
    preUpgradeSnapshot: true
```

2. **Configure the upgrade manager to use the role:**

```yaml
spec:
  upgrade:
    jwtAuthRole: upgrade
```

**Note:** For `Hardened` profile clusters, JWT authentication is automatically bootstrapped during self-init, so you only need to create the upgrade policy and role.

#### 12.1.2 Static Token (Alternative)

For clusters that cannot use JWT Auth, you must create a token Secret with appropriate permissions and reference it via `spec.upgrade.tokenSecretRef`:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: upgrade-cluster
spec:
  upgrade:
    tokenSecretRef:
      name: upgrade-token-secret
      namespace: openbao-operator-system
    preUpgradeSnapshot: true
```

The token must have permission to:
- Read `sys/health`
- Update `sys/step-down`
- Read `sys/storage/raft/snapshot` (if `preUpgradeSnapshot` is enabled)

### 12.2 Upgrade ServiceAccount

The operator automatically creates `<cluster-name>-upgrade-serviceaccount` when upgrade authentication is configured. This ServiceAccount:
- Is used by the upgrade manager for JWT Auth (via projected ServiceAccount token)
- Has its token automatically obtained via projected volume with audience `openbao-internal`
- Is owned by the OpenBaoCluster for automatic cleanup

### 12.3 Performing Upgrades

To upgrade an OpenBao cluster, update the `spec.version` field:

```sh
kubectl -n security patch openbaocluster dev-cluster \
  --type merge -p '{"spec":{"version":"2.5.0"}}'
```

The upgrade manager will:
1. Validate the target version
2. Perform pre-upgrade validation (cluster health, quorum, leader detection)
3. Create a pre-upgrade snapshot (if `spec.upgrade.preUpgradeSnapshot` is enabled)
4. Perform a rolling upgrade, pod-by-pod, with leader step-down handling
5. Update `Status.CurrentVersion` when complete

Monitor upgrade progress:

```sh
kubectl -n security get openbaocluster dev-cluster -o yaml
```

Check `Status.Upgrade` for current progress and `Status.Conditions` for upgrade state.
