# Security Profiles

<Callout type="abstract" title="Concept">

OpenBao Operator supports two distinct security profiles via `spec.profile`. These profiles enforce different validation rules and default behaviors to match the environment's risk level.

</Callout>

## Profile Comparison

| Feature | Hardened (Production) | Development (Testing) |
| :--- | :--- | :--- |
| **Root Token** | Auto-revoked (not stored in a Secret) | Stored in a Secret when self-init is disabled |
| **Unseal Keys** | Non-static external root of trust required | Defaults to static key stored in a Secret |
| **TLS** | External / ACME required | Operator-managed allowed |
| **Replicas** | Minimum 3 (HA Required) | Any (1+) |
| **Self-Init** | Required (`enabled=true`) | Optional |
| **Admission Check** | Strict Validation | Relaxed Validation |
| **Supply Chain** | Verification blocks cannot be disabled; digest-only admission applies to managed workloads | Verification optional |
| **Use Case** | **Production** | Proof of Concept, Local Dev |

## Detailed Configuration

<Tabs groupId="hardened-profile-development-profile">

<TabItem value="hardened-profile" label="Hardened Profile">

<Callout type="success" title="Production Ready">

The `Hardened` profile is **MANDATORY** for production deployments. It is the supported production posture for OpenBao Operator and enforces a secure-by-default bootstrap and runtime model.

</Callout>

To use this profile, your `OpenBaoCluster` must meet these requirements:

1.  **High Availability:** You must set `spec.replicas` to at least `3` for Raft quorum.
2.  **External Root of Trust:** You must use a non-static unseal backend such as `transit`, `awskms`, `gcpckms`, `azurekeyvault`, `ocikms`, `kmip`, or `pkcs11`.
3.  **Valid TLS:** You must provide valid TLS certificates. `OperatorManaged` TLS is rejected for `Hardened` clusters by admission policy.
4.  **Self-Initialization:** You must enable self-init. Manual bootstrap is not a supported production path because it persists a root token Secret.
5.  **Image Verification Guardrails:** You cannot set `spec.imageVerification.enabled=false`, `spec.operatorImageVerification.enabled=false`, or use `failurePolicy: Warn`.

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: production-cluster
spec:
  profile: Hardened
  replicas: 3 # Minimum 3 for HA
  tls:
    enabled: true
    mode: External # or ACME
  selfInit:
    enabled: true
  unseal:
    type: awskms # or transit, gcpckms, azurekeyvault, ocikms, kmip, pkcs11
```

If image verification blocks are omitted in `Hardened`, the operator still treats verification as enabled.
Official release images receive default keyless identity settings; custom/air-gapped registries should
provide explicit `publicKey` or keyless identity fields.

</TabItem>

<TabItem value="development-profile" label="Development Profile">

<Callout type="warning" title="Non-Production Only">

The `Development` profile is highly discouraged for production. It creates significant security risks by allowing bootstrap and unseal material to be stored in Kubernetes Secrets.

</Callout>

This profile is useful for:

-   Local testing (Minikube/Kind).
-   CI/CD integration tests.
-   Rapid prototyping where long-term security is not required.

**Key Behaviors:**

-   **Root Token:** Stored in `<cluster-name>-root-token` when self-init is disabled.
-   **Unseal Keys:** Stored in `<cluster-name>-unseal-key` when `spec.unseal.type` is `static` (default).
-   **Status Warning:** The Operator sets `ConditionSecurityRisk=True` on the cluster status.

<Callout type="tip" title="Prefer Self-Init">

Even in Development, enabling `spec.selfInit.enabled: true` avoids root token Secret creation. Do not store raw secrets in Git.

</Callout>

</TabItem>

</Tabs>

## Guidance

<Callout type="tip" title="Migration Path">

Teams often start with **Development** for initial exploration. When moving to **Staging** or **Production**, create a *new* cluster with the **Hardened** profile rather than trying to convert an existing Development cluster. Trust roots established in Development are typically not secure enough for Production.

</Callout>

## See Also

- [Infrastructure Security](../infrastructure/index.md)
- [Server Configuration](../../user-guide/openbaocluster/configuration/server.md)

