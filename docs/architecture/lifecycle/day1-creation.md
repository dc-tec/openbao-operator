# Day 1: Cluster Creation

Day 1 involves the instantiation and initialization of the OpenBao cluster itself.

!!! tip "User Guide"
    For details on configuring the cluster, see the [Cluster Configuration](../../user-guide/openbaocluster/getting-started.md) and [Security Fundamentals](../../security/fundamentals/index.md).

## Initialization Strategies

=== "Self-Initialization (Recommended)"

    When `spec.selfInit.enabled = true`, the cluster uses OpenBao's native self-initialization feature:

    1. User creates an `OpenBaoCluster` CR with `spec.selfInit` configured.
    2. Cert Manager (workload controller) bootstraps PKI (CA + leaf certs).
    3. If image verification is enabled, the operator verifies the container image signature before proceeding.
    4. Infrastructure Manager (workload controller) ensures a per-cluster auto-unseal configuration (static seal by default, or external KMS if configured).
    5. Infrastructure Manager renders `config.hcl` with TLS, Raft storage, `retry_join`, `service_registration "kubernetes"`, and the appropriate `seal` stanza.
       - The init container appends the self-init `initialize` stanzas (rendered from `spec.selfInit.requests[]`) to the rendered config for pod-0 only.
    6. Infrastructure Manager creates the StatefulSet with **1 replica initially**.
    7. OpenBao automatically initializes itself on first start using the `initialize` stanzas:
       - Auto-unseals using the configured seal (static or external KMS).
       - Executes all configured `initialize` requests (audit, auth, secrets, policies).
       - **The root token is NOT returned and is automatically revoked after use.**
    8. Init Manager detects initialization via Kubernetes service registration labels (preferred) and sets `status.initialized = true`.
       It also sets `status.selfInitialized = true`.
    9. Infrastructure Manager scales the StatefulSet to the desired `spec.replicas`.
    10. Additional OpenBao pods start, auto-unseal, and join the Raft cluster.

    !!! note "Self-Initialization vs Root Token"
        Self-initialization requires an auto-unseal mechanism (which the Operator provides via static auto-unseal by default, or external KMS if configured). No root token Secret is created when self-init is enabled.

    !!! warning "Version Requirement"
        The static auto-unseal feature requires **OpenBao v2.4.0 or later**. Earlier versions do not support the `seal "static"` configuration. External KMS seals may have different version requirements depending on the provider.

    **Sequence Diagram:**

    ```mermaid
    sequenceDiagram
        autonumber
        participant U as User
        participant K as Kubernetes API
        participant Op as OpenBao Operator
        participant Bao as OpenBao Pods

        U->>K: Create OpenBaoCluster (selfInit.enabled=true)
        K-->>Op: Watch OpenBaoCluster
        Op->>K: Create TLS Secrets (CA, server)
        Op->>K: Create config ConfigMap (config.hcl)
        Op->>K: Create self-init ConfigMap (initialize blocks)
        Op->>K: Create StatefulSet (replicas=1)
        Bao-->>Bao: Auto-init and run initialize requests
        Bao-->>Bao: Auto-unseal using static or KMS seal
        Bao-->>K: Update Pod labels (service_registration)
        Op->>K: Observe pod-0 labels (initialized/sealed)
        Op->>K: Update OpenBaoCluster.status.initialized=true
        Op->>K: Scale StatefulSet to spec.replicas
        Bao-->>Op: Additional pods auto-unseal and join Raft
    ```

=== "Standard Initialization"

    1. User creates an `OpenBaoCluster` CR in a namespace (without `spec.selfInit`).
    2. Cert Manager (workload controller) bootstraps PKI (CA + leaf certs).
    3. If image verification is enabled (`spec.imageVerification.enabled`), the operator verifies the container image signature using Cosign before proceeding.
    4. Infrastructure Manager (workload controller) ensures a per-cluster auto-unseal configuration:
       - If `spec.unseal` is omitted or `spec.unseal.type` is `"static"`, creates a static auto-unseal key Secret (`<cluster>-unseal-key`) if missing.
       - If `spec.unseal.type` is an external KMS provider (`awskms`, `gcpckms`, `azurekeyvault`, `transit`), configures the seal with the provided options and credentials (if specified).
    5. Infrastructure Manager renders `config.hcl` with TLS, Raft storage, `retry_join`, `service_registration "kubernetes"`, and the appropriate `seal` stanza (static or external KMS).
       - The Operator injects `BAO_K8S_NAMESPACE` into the OpenBao container environment so service registration can determine the Pod namespace.
    6. Infrastructure Manager creates the StatefulSet with **1 replica initially** (regardless of `spec.replicas`), mounting TLS and unseal Secrets (if using static seal) or KMS credentials (if using external KMS).
    7. Init Manager waits for pod-0 to be running, then:
       - Prefers Kubernetes service registration Pod labels (`openbao-initialized`, `openbao-sealed`) when available.
       - Falls back to the HTTP health endpoint (`GET /v1/sys/health`) when labels are not yet available.
       - If not, calls the HTTP initialization endpoint (`PUT /v1/sys/init`) against pod-0 to initialize the cluster.
       - Stores the root token in a per-cluster Secret (`<cluster>-root-token`).
       - Sets `status.initialized = true`.
    8. Once initialized, Infrastructure Manager scales the StatefulSet to the desired `spec.replicas`.
    9. Additional OpenBao pods start, auto-unseal using the static key, join the Raft cluster via `retry_join`, and become Ready.

    **Sequence Diagram:**

    ```mermaid
    sequenceDiagram
        autonumber
        participant U as User
        participant K as Kubernetes API
        participant Op as OpenBao Operator
        participant Bao as OpenBao Pods

        U->>K: Create OpenBaoCluster (no selfInit)
        K-->>Op: Watch OpenBaoCluster
        Op->>K: Create TLS Secrets (CA, server)
        Op->>K: Create config ConfigMap (config.hcl)
        Op->>K: Create StatefulSet (replicas=1)
        Bao-->>K: Update Pod labels (service_registration)
        Op->>K: Observe pod-0 labels (initialized/sealed)
        alt labels not yet available
            Op->>Bao: Call /v1/sys/health (check initialized)
        end
        Op->>Bao: Call /v1/sys/init (initialize cluster)
        Op->>K: Store root token Secret
        Op->>K: Update OpenBaoCluster.status.initialized=true
        Op->>K: Scale StatefulSet to spec.replicas
        Bao-->>Op: Pods auto-unseal and join Raft
    ```
