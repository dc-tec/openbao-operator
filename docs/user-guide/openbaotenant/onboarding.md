# Tenant Onboarding & Governance

Before creating an `OpenBaoCluster`, the target namespace must be provisioned with the necessary RBAC. The operator supports two governance models: **Self-Service** (decentralized) and **Centralized Admin** (strict control).

=== ":material-account-cog: Self-Service (Recommended)"

    In this model, namespace owners can onboard themselves without cluster-admin intervention. This relies on the `Confused Deputy` prevention logic: users can only provision the namespace they already have access to.

    ### Prerequisites

    Ensure the `openbaotenant-editor-role` is bound to your user (this is aggregated to the standard `admin` and `edit` ClusterRoles by default).

    ### Steps {: #self-service-onboarding }

    1. Create an `OpenBaoTenant` resource **in your own namespace**, targeting **that same namespace**:

        ```yaml
        apiVersion: openbao.org/v1alpha1
        kind: OpenBaoTenant
        metadata:
          name: my-tenant-onboarding
          namespace: team-a-prod  # (1)!
        spec:
          targetNamespace: team-a-prod # (2)!
        ```

        1.  Your namespace.
        2.  MUST match metadata.namespace.

    2. Apply the resource:

        ```sh
        kubectl apply -f my-tenant.yaml
        ```

    3. The Provisioner controller will detect this valid request and create the necessary `Role` and `RoleBinding` in `team-a-prod` to allow the operator to manage resources.

    ### Security Note

    If you attempt to target a different namespace (e.g., `targetNamespace: kube-system`), the controller will **block** the request and update the status with a `SecurityViolation` error.

=== ":material-police-badge-outline: Centralized Admin"

    In this model, cluster administrators explicitly declare which namespaces are valid tenants. This is useful for strict environments where users should not self-provision.

    ### Steps {: #centralized-admin-onboarding }

    1. As a cluster administrator, create an `OpenBaoTenant` resource in the **operator's namespace** (typically `openbao-operator-system`):

        ```yaml
        apiVersion: openbao.org/v1alpha1
        kind: OpenBaoTenant
        metadata:
          name: team-b-authorization
          namespace: openbao-operator-system # (1)!
        spec:
          targetNamespace: team-b-prod      # (2)!
        ```

        1.  Trusted namespace.
        2.  Can be any namespace.

    2. Since the request originates from the trusted operator namespace, the controller allows cross-namespace provisioning.

## 3. Verifying Provisioning

Check the `OpenBaoTenant` status:

```sh
kubectl -n team-a-prod get openbaotenant my-tenant-onboarding -o yaml
```

Look for:

* `status.provisioned: true`: RBAC successfully applied.
* `status.lastError`: detailed error message if provisioning failed.
* **Conditions**:
  * `Type: Provisioned`, `Status: False`, `Reason: SecurityViolation`: You attempted an unauthorized cross-namespace provisioning.

## 4. How It Works (Security Model)

The operator uses a **Trust-But-Verify** approach:

1. **Trust**: The Operator's own namespace (`openbao-operator-system`) is trusted. Resources created there can target *any* namespace.
2. **Verify**: Resources created in user namespaces are verified. They must target their own namespace (`metadata.namespace == spec.targetNamespace`).
3. **Isolation**: The Provisioner uses a delegated ServiceAccount with minimal permissions. It cannot list all namespaces in the cluster; it only acts on namespaces explicitly discovered via valid `OpenBaoTenant` CRs.
