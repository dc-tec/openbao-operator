# 16. Multi-Tenancy Security

[Back to User Guide index](README.md)

When running multiple `OpenBaoCluster` resources in a shared Kubernetes cluster (multi-tenant), additional security considerations apply.

### 16.1 RBAC for Tenant Isolation

The operator provides ClusterRoles that can be bound at the namespace level to isolate tenant access:

```sh
# See example RBAC configurations
kubectl apply -f config/samples/namespace_scoped_openbaocluster-rbac.yaml
```

**Key points:**

- Use `RoleBinding` (not `ClusterRoleBinding`) to restrict users to their namespace
- The `openbaocluster-editor-role` allows creating/managing clusters but NOT reading Secrets
- Platform teams should have `openbaocluster-admin-role` cluster-wide

### 16.2 Secret Access Isolation

The operator creates sensitive Secrets for each cluster:
- `<cluster>-root-token` - Full admin access to OpenBao
- `<cluster>-unseal-key` - Can decrypt all OpenBao data
- `<cluster>-tls-ca` / `<cluster>-tls-server` - TLS credentials

**Critical:** Tenants should NOT have direct access to these Secrets. Configure RBAC to restrict Secret access:

```yaml
# Example: Deny Secret access in tenant namespace
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: deny-openbao-secrets
  namespace: team-a-prod
rules:
  # Explicitly grant access only to non-sensitive secrets
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list"]
    resourceNames: []  # Empty = no access to any secrets by name
---
# For production, consider using OPA/Gatekeeper or Kyverno to enforce
# that tenants cannot read secrets matching *-root-token, *-unseal-key patterns
```

**Recommendation:** Use a policy engine (OPA Gatekeeper, Kyverno) to enforce Secret access restrictions:

```yaml
# Example Kyverno policy to block access to sensitive OpenBao secrets
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: block-openbao-sensitive-secrets
spec:
  validationFailureAction: Enforce
  background: false
  rules:
    - name: block-root-token-access
      match:
        any:
          - resources:
              kinds:
                - Secret
              names:
                - "*-root-token"
                - "*-unseal-key"
      exclude:
        any:
          - subjects:
              - kind: ServiceAccount
                name: openbao-operator-controller-manager
                namespace: openbao-operator-system
      validate:
        message: "Access to OpenBao root token and unseal key secrets is restricted"
        deny: {}
```

### 16.3 Network Policies

The operator automatically creates a NetworkPolicy for each OpenBaoCluster that enforces default-deny ingress and restricts egress. The operator auto-detects the Kubernetes API server CIDR by querying the `kubernetes` Service in the `default` namespace. If auto-detection fails (e.g., due to restricted permissions in multi-tenant environments), you can manually configure `spec.network.apiServerCIDR` as a fallback (see [Network Configuration](#101-api-server-cidr-fallback)).

**Backup job pods are excluded from this NetworkPolicy** (they have the label `openbao.org/component=backup`) to allow access to object storage services.

If you need to restrict backup job network access, create a custom NetworkPolicy targeting backup jobs:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: backup-job-network-policy
  namespace: team-a-prod
spec:
  podSelector:
    matchLabels:
      openbao.org/component: backup
      openbao.org/cluster: team-a-cluster
  policyTypes:
    - Ingress
    - Egress
  ingress:
    # Allow ingress from operator (if needed for monitoring)
    - from:
        - namespaceSelector:
            matchLabels:
              app.kubernetes.io/name: openbao-operator
      ports:
        - port: 8080  # metrics port
  egress:
    # Allow DNS
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: kube-system
      ports:
        - port: 53
          protocol: UDP
        - port: 53
          protocol: TCP
    # Allow access to object storage (adjust namespace/port as needed)
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: rustfs
      ports:
        - port: 9000
          protocol: TCP
    # Allow access to OpenBao cluster for snapshot API
    - to:
        - podSelector:
            matchLabels:
              openbao.org/cluster: team-a-cluster
      ports:
        - port: 8200
          protocol: TCP
```

For general cluster isolation, the operator-managed NetworkPolicy provides:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: openbao-cluster-isolation
  namespace: team-a-prod
spec:
  podSelector:
    matchLabels:
      openbao.org/cluster: team-a-cluster
  policyTypes:
    - Ingress
    - Egress
  ingress:
    # Allow traffic only from pods in the same cluster
    - from:
        - podSelector:
            matchLabels:
              openbao.org/cluster: team-a-cluster
      ports:
        - port: 8200
        - port: 8201
    # Allow traffic from ingress controller (if needed)
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: ingress-nginx
      ports:
        - port: 8200
  egress:
    # Allow cluster-internal communication
    - to:
        - podSelector:
            matchLabels:
              openbao.org/cluster: team-a-cluster
      ports:
        - port: 8200
        - port: 8201
    # Allow DNS
    - to:
        - namespaceSelector: {}
          podSelector:
            matchLabels:
              k8s-app: kube-dns
      ports:
        - port: 53
          protocol: UDP
    # Allow Kubernetes API (for auto-join discovery via discover-k8s provider)
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: default
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: kube-system
      ports:
        - port: 443
          protocol: TCP
```

### 16.4 Resource Quotas

Prevent one tenant from exhausting cluster resources:

```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: openbao-quota
  namespace: team-a-prod
spec:
  hard:
    # Limit number of OpenBao pods
    pods: "10"
    # Limit total PVC storage
    requests.storage: "100Gi"
    persistentvolumeclaims: "5"
    # Limit CPU/Memory
    requests.cpu: "4"
    requests.memory: "8Gi"
    limits.cpu: "8"
    limits.memory: "16Gi"
```

### 16.5 Backup Credential Isolation

Each tenant should have **separate backup credentials** with access only to their backup prefix:

```yaml
# Tenant A backup credentials - access only to tenant-a/ prefix
apiVersion: v1
kind: Secret
metadata:
  name: backup-credentials-team-a
  namespace: team-a-prod
type: Opaque
stringData:
  accessKeyId: "TENANT_A_ACCESS_KEY"
  secretAccessKey: "TENANT_A_SECRET_KEY"
  # Region is optional
```

Configure the S3/object storage IAM policy to restrict access:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::openbao-backups/team-a-prod/*",
        "arn:aws:s3:::openbao-backups"
      ],
      "Condition": {
        "StringLike": {
          "s3:prefix": ["team-a-prod/*"]
        }
      }
    }
  ]
}
```

Then reference in the OpenBaoCluster:

```yaml
spec:
  backup:
    schedule: "0 3 * * *"
    target:
      endpoint: "https://s3.amazonaws.com"
      bucket: "openbao-backups"
      pathPrefix: "team-a-prod/"  # Isolated prefix per tenant
      credentialsSecretRef:
        name: backup-credentials-team-a
        namespace: team-a-prod
```

### 16.6 Pod Security Standards

OpenBao pods created by the operator run with these security settings:
- Non-root user (UID 100, GID 1000)
- Read-only root filesystem (secrets/config mounted read-only)
- No privilege escalation

Ensure your cluster's Pod Security Admission is configured to allow these pods:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: team-a-prod
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/warn: restricted
```

### 16.7 Operator Audit Logging

The Operator emits structured audit logs for critical operations, tagged with `audit=true` and `event_type` for easy filtering in log aggregation systems. Audit events are logged for:

- **Cluster Initialization:** `Init`, `InitCompleted`, `InitFailed`
- **Leader Step-Down:** `StepDown`, `StepDownCompleted`, `StepDownFailed` (during upgrades)

These audit logs are distinct from regular debug/info logs and can be filtered using the `audit=true` label.

### 16.8 Kubernetes Audit Logging

Enable Kubernetes audit logging to monitor access to sensitive resources:

```yaml
# Example audit policy for OpenBao-related resources
apiVersion: audit.k8s.io/v1
kind: Policy
rules:
  # Log all access to OpenBao secrets
  - level: RequestResponse
    resources:
      - group: ""
        resources: ["secrets"]
    namespaces: ["*"]
    verbs: ["get", "list", "watch"]
    omitStages:
      - RequestReceived
  # Log all OpenBaoCluster changes
  - level: RequestResponse
    resources:
      - group: "openbao.org"
        resources: ["openbaoclusters"]
    verbs: ["create", "update", "patch", "delete"]
```

### 16.9 Etcd Encryption Warning

The Operator sets an `EtcdEncryptionWarning` condition to remind users that security relies on underlying Kubernetes secret encryption at rest. The operator cannot verify etcd encryption status, so this condition is always set to `True` with a message reminding users to ensure etcd encryption is enabled.

Check the condition:

```sh
kubectl -n security get openbaocluster dev-cluster -o jsonpath='{.status.conditions[?(@.type=="EtcdEncryptionWarning")]}'
```

### 16.10 Multi-Tenancy Checklist

Before deploying OpenBao in a multi-tenant environment, verify:

- [ ] `OpenBaoTenant` CRDs are created for each tenant namespace (only cluster administrators should have permission to create these)
- [ ] Namespace-scoped RoleBindings configured for each tenant (automatically created by Provisioner)
- [ ] Tenants cannot read `*-root-token` and `*-unseal-key` Secrets
- [ ] NetworkPolicies isolate OpenBao clusters from each other (operator-managed NetworkPolicy excludes backup jobs; create custom NetworkPolicy for backup jobs if restrictions are needed)
- [ ] ResourceQuotas limit tenant resource consumption
- [ ] Each tenant has separate backup credentials with isolated bucket prefixes
- [ ] Pod Security Admission configured appropriately (automatically applied by Provisioner)
- [ ] Kubernetes audit logging enabled for Secret access and `OpenBaoTenant` CRD access
- [ ] etcd encryption at rest enabled for Secrets (operator will show `EtcdEncryptionWarning` condition if status cannot be verified)
- [ ] Consider using self-initialization to avoid root token Secrets entirely
- [ ] Operator-managed NetworkPolicy is in place (automatically created by the Operator)
- [ ] Container images are pinned to specific versions (not using `:latest` tags)
- [ ] Operator audit logs are being collected and monitored (filter by `audit=true` label)
