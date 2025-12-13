# 3. Provision Tenant Namespaces

[Back to User Guide index](README.md)

Before creating an `OpenBaoCluster`, you must provision the target namespace with tenant RBAC. The operator uses a governance model based on the `OpenBaoTenant` Custom Resource Definition (CRD) to explicitly declare which namespaces should be provisioned.

### 3.1 Create an OpenBaoTenant Resource

Create an `OpenBaoTenant` resource in the operator's namespace (typically `openbao-operator-system`) to declare a target namespace for provisioning:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoTenant
metadata:
  name: security-tenant
  namespace: openbao-operator-system
spec:
  targetNamespace: security
```

**Important Notes:**

- The `OpenBaoTenant` resource must be created in the operator's namespace (where the Provisioner controller runs).
- The `spec.targetNamespace` field specifies the namespace that will receive tenant RBAC.
- The target namespace must exist before the Provisioner can successfully provision it. If the namespace doesn't exist, the Provisioner will update `Status.LastError` and retry periodically.
- Only cluster administrators should have permission to create `OpenBaoTenant` resources, as this grants the OpenBaoCluster controller access to the target namespace.

### 3.2 Verify Provisioning

Check the `OpenBaoTenant` status to verify provisioning:

```sh
kubectl -n openbao-operator-system get openbaotenant security-tenant -o yaml
```

Look for:
- `status.provisioned: true` - Indicates RBAC has been successfully applied
- `status.lastError` - Any errors encountered during provisioning (e.g., namespace not found)

Verify that the tenant Role and RoleBinding were created:

```sh
kubectl -n security get role,rolebinding -l app.kubernetes.io/component=provisioner
```

You should see:
- `openbao-operator-tenant-role` - Namespace-scoped Role granting OpenBaoCluster management permissions
- `openbao-operator-tenant-rolebinding` - RoleBinding binding the controller ServiceAccount to the Role

### 3.3 Security Benefits

The `OpenBaoTenant` governance model provides significant security improvements:

- **No Namespace Enumeration:** The Provisioner cannot list or watch namespaces, preventing it from discovering cluster topology even if compromised.
- **Explicit Declaration:** Only namespaces explicitly declared in `OpenBaoTenant` CRDs receive tenant RBAC, reducing the attack surface.
- **Access Control:** Creating `OpenBaoTenant` resources requires write access to the operator's namespace, which should be restricted to cluster administrators.

### 3.4 Cleanup

When you no longer need a tenant namespace, delete the `OpenBaoTenant` resource:

```sh
kubectl -n openbao-operator-system delete openbaotenant security-tenant
```

The Provisioner will automatically clean up the tenant Role and RoleBinding from the target namespace when the `OpenBaoTenant` is deleted.
