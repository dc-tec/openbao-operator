# Security Profiles

!!! abstract "Concept"
    OpenBao Operator supports two distinct security profiles via `spec.profile`. These profiles enforce different validation rules and default behaviors to match the environment's risk level.

## Profile Comparison

| Feature | :material-shield-check: Hardened (Production) | :material-test-tube: Development (Testing) |
| :--- | :--- | :--- |
| **Root Token** | Auto-revoked (not stored in a Secret) | Stored in a Secret when self-init is disabled |
| **Unseal Keys** | External KMS required | Defaults to static key stored in a Secret |
| **TLS** | External / ACME required | Operator-managed allowed |
| **Replicas** | Minimum 3 (HA Required) | Any (1+) |
| **Self-Init** | Required (`enabled=true`) | Optional |
| **Admission Check** | Strict Validation | Relaxed Validation |
| **Use Case** | **Production** | Proof of Concept, Local Dev |

## Detailed Configuration

=== ":material-shield-check: Hardened Profile"

    !!! success "Production Ready"
        The `Hardened` profile is **MANDATORY** for production deployments. It enforces a "secure by default" posture that prevents root-token Secret creation and requires strong encryption.

    To use this profile, your `OpenBaoCluster` must meet these requirements:

    1.  **High Availability:** You must set `spec.replicas` to at least `3` for Raft quorum.
    2.  **External KMS:** You must provide a KMS key (AWS, GCP, Azure, or Vault Transit) for auto-unseal.
    3.  **Valid TLS:** You must provide valid TLS certificates; insecure TLS verification skips are rejected.
    4.  **Self-Initialization:** You must enable self-init to avoid persisting a root token Secret.

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
        type: awskms # or gcpckms, azurekeyvault, transit
    ```

=== ":material-test-tube: Development Profile"

    !!! warning "Non-Production Only"
        The `development` profile creates significant security risks by storing the **Root Token** in a Kubernetes Secret. This allows any user with Secret read permissions to take full control of the cluster.

    This profile is useful for:
    
    -   Local testing (Minikube/Kind).
    -   CI/CD integration tests.
    -   Rapid prototyping where long-term security is not required.

    **Key Behaviors:**
    
    -   **Root Token:** Stored in `<cluster-name>-root-token` when self-init is disabled.
    -   **Unseal Keys:** Stored in `<cluster-name>-unseal-key` when `spec.unseal.type` is `static` (default).
    -   **Status Warning:** The Operator sets `ConditionSecurityRisk=True` on the cluster status.

    !!! tip "Prefer Self-Init"
        Even in Development, enabling `spec.selfInit.enabled: true` avoids root token Secret creation. Do not store raw secrets in Git.

## Guidance

!!! tip "Migration Path"
    Teams often start with **Development** for initial exploration. When moving to **Staging** or **Production**, create a *new* cluster with the **Hardened** profile rather than trying to convert an existing Development cluster. Trust roots established in Development are typically not secure enough for Production.

## See Also

- [:material-server-network: Infrastructure Security](../infrastructure/index.md)
- [Server Configuration](../../user-guide/openbaocluster/configuration/server.md)
