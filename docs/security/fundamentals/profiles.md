# Security Profiles

!!! abstract "Concept"
    OpenBao Operator supports two distinct security profiles via `spec.profile`. These profiles enforce different validation rules and default behaviors to match the environment's risk level.

## Profile Comparison

| Feature | :material-shield-check: Hardened (Production) | :material-test-tube: Development (Testing) |
| :--- | :--- | :--- |
| **Root Token** | **Never Generated** | Stored in Secret |
| **Unseal Keys** | External KMS Required | Stored in Secret |
| **TLS** | External / ACME Required | Operator Managed Allowed |
| **Replicas** | Minimum 3 (HA Required) | Any (1+) |
| **Self-Init** | Required (`enabled=true`) | Optional |
| **Admission Check** | Strict Validation | Relaxed Validation |
| **Use Case** | **Production** | Proof of Concept, Local Dev |

## Detailed Configuration

=== ":material-shield-check: Hardened Profile"

    !!! success "Production Ready"
        The `hardened` profile is **MANDATORY** for all production deployments. It enforces a "Secure by Default" posture that eliminates Root Tokens and ensures strong encryption.

    To use this profile, your `OpenBaoCluster` must meet these requirements:

    1.  **High Availability:** You must set `spec.replicas` to at least `3` for Raft quorum.
    2.  **External KMS:** You must provide a KMS key (AWS, GCP, Azure, or Vault Transit) for auto-unseal.
    3.  **Valid TLS:** You must provide valid TLS certificates (via `cert-manager` or external secret); `tlsSkipVerify` is rejected.
    4.  **Self-Initialization:** The Operator must drive the initialization process to ensure no humans handle initial secrets.

    ```yaml
    apiVersion: openbao.org/v1alpha1
    kind: OpenBaoCluster
    metadata:
      name: production-cluster
    spec:
      profile: hardened
      replicas: 3  # Minimum 3 for HA
      selfInit:
        enabled: true
      unseal:
        mode: awskms # or gcpckms, azurekeyvault
    ```

=== ":material-test-tube: Development Profile"

    !!! warning "Non-Production Only"
        The `development` profile creates significant security risks by storing the **Root Token** in a Kubernetes Secret. This allows any user with Secret read permissions to take full control of the cluster.

    This profile is useful for:
    
    -   Local testing (Minikube/Kind).
    -   CI/CD integration tests.
    -   Rapid prototyping where long-term security is not required.

    **Key Behaviors:**
    
    -   **Root Token:** Generated and stored in `<cluster-name>-root-token`.
    -   **Unseal Keys:** Generated and stored in `<cluster-name>-unseal-key` (unless KMS is configured).
    -   **Status Warning:** The Operator sets `ConditionSecurityRisk=True` on the cluster status.

## Guidance

!!! tip "Migration Path"
    Teams often start with **Development** for initial exploration. When moving to **Staging** or **Production**, you should create a *new* cluster with the **Hardened** profile rather than trying to converting an existing Development cluster. Trust roots established in Development are typically not secure enough for Production.

## See Also

- [:material-server-network: Infrastructure Security](../infrastructure/index.md)
- [Server Configuration](../../user-guide/openbaocluster/configuration/server.md)
