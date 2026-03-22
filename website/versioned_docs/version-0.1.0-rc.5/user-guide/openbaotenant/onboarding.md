# Tenant Onboarding & Governance

Before creating an `OpenBaoCluster`, the target namespace must be provisioned with the necessary RBAC. The operator supports two governance models: **Self-Service** (decentralized) and **Centralized Admin** (strict control).

<Tabs groupId="self-service-recommended-centralized-admin">

<TabItem value="self-service-recommended" label="Self-Service (Recommended)">

In this model, namespace owners can onboard themselves without cluster-admin intervention. This relies on the `Confused Deputy` prevention logic: users can only provision the namespace they already have access to.

### Prerequisites

Ensure the `openbaotenant-editor-role` is bound to your user (this is aggregated to the standard `admin` and `edit` ClusterRoles by default).

### Self-Service Onboarding

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

Self-service onboarding may not set `spec.quota` or `spec.limitRange`. The operator-owned tenant guardrails use the default values unless a cluster administrator creates the `OpenBaoTenant` from the operator namespace.

2. Apply the resource:

    ```sh
    kubectl apply -f my-tenant.yaml
    ```

3. The Provisioner controller will detect this valid request and create the necessary `Role` and `RoleBinding` in `team-a-prod` to allow the operator to manage resources.

### Security Note

If you attempt to target a different namespace (e.g., `targetNamespace: kube-system`), the controller will **block** the request and update the status with a `SecurityViolation` error.

</TabItem>

<TabItem value="centralized-admin" label="Centralized Admin">

In this model, cluster administrators explicitly declare which namespaces are valid tenants. This is useful for strict environments where users should not self-provision.

### Centralized Admin Onboarding

1. As a cluster administrator, create an `OpenBaoTenant` resource in the **operator's namespace**:

    ```yaml
    apiVersion: openbao.org/v1alpha1
    kind: OpenBaoTenant
    metadata:
      name: team-b-authorization
      namespace: <operator-namespace> # (1)!
    spec:
      targetNamespace: team-b-prod      # (2)!
    ```

    1.  Rendered operator namespace. Default raw-manifest and Helm installs use `openbao-operator-system`.
    2.  Can be any namespace.

2. Since the request originates from the trusted operator namespace, the controller allows cross-namespace provisioning.

This is also the supported path for custom tenant guardrails. Cluster administrators may set `spec.quota` and `spec.limitRange` when they need tighter or larger defaults for a specific namespace.

</TabItem>

</Tabs>

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

1. **Trust**: The operator's rendered namespace is trusted. Resources created there can target *any* namespace.
2. **Verify**: Resources created in user namespaces are verified. They must target their own namespace (`metadata.namespace == spec.targetNamespace`).
3. **Isolation**: The Provisioner uses a delegated ServiceAccount with minimal permissions. It cannot list all namespaces in the cluster; it only acts on namespaces explicitly discovered via valid `OpenBaoTenant` CRs.

<Callout type="note" title="API Contract">

`spec.targetNamespace` is immutable after creation. To change the target namespace, delete and recreate the `OpenBaoTenant`.

</Callout>
