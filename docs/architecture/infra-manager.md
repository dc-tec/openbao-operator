# InfrastructureManager (Config & StatefulSet)

**Responsibility:** Render configuration and maintain the StatefulSet.

## Configuration Strategy

- We do not use a static ConfigMap. We generate it dynamically based on the cluster topology and merge in user-supplied configuration where safe.
- **Injection:** We explicitly inject the `listener "tcp"` block:
  - For `OperatorManaged` and `External` modes: Points to mounted TLS Secrets at `/etc/bao/tls`.
  - For `ACME` mode: Includes ACME parameters (`tls_acme_ca_directory`, `tls_acme_domains`, etc.) and no file paths.

## Discovery Strategy

- **Bootstrap:** During initial cluster creation, we configure a single `retry_join` with `leader_api_addr` pointing to the deterministic DNS name of pod-0 (for example, `cluster-0.cluster-name.namespace.svc`). This ensures a stable leader for Day 0 initialization.
- **Post-Initialization:** After the cluster is marked initialized, we switch to a Kubernetes go-discover based `retry_join` that uses `auto_join = "provider=k8s namespace=<ns> label_selector=\"openbao.org/cluster=<cluster-name>\""` so new pods can join dynamically based on labels rather than a static peer list.

## Auto-Unseal Integration

### Static Seal (Default)

If `spec.unseal` is omitted or `spec.unseal.type` is `"static"`:

- On first reconcile, checks for the existence of `Secret/<cluster>-unseal-key` (per-cluster name).
- If missing, generates 32 cryptographically secure random bytes and stores them as the unseal key in the Secret.
- Mounts this Secret into the StatefulSet PodSpec at `/etc/bao/unseal/key`.
- Injects a `seal "static"` stanza into `config.hcl`.

### External KMS Seal

If `spec.unseal.type` is set to `"awskms"`, `"gcpckms"`, `"azurekeyvault"`, `"transit"`, `"kmip"`, `"ocikms"`, or `"pkcs11"`:

- Does NOT create the `<cluster>-unseal-key` Secret.
- Renders a `seal "<type>"` block with structured configuration from the corresponding seal config (e.g., `spec.unseal.transit`, `spec.unseal.awskms`, etc.).
- If `spec.unseal.credentialsSecretRef` is provided, mounts the credentials Secret at `/etc/bao/seal-creds`.
- For GCP Cloud KMS (`gcpckms`), sets `GOOGLE_APPLICATION_CREDENTIALS` environment variable pointing to the mounted credentials file.

## Image Verification (Supply Chain Security)

If `spec.imageVerification.enabled` is `true`, the operator verifies the container image signature using Cosign before creating or updating the StatefulSet.

- The verification uses the public key provided in `spec.imageVerification.publicKey`.
- Verification results are cached in-memory to avoid redundant network calls for the same image digest.

**Failure Policy:**

| Policy | Behavior |
| ------ | -------- |
| `Block` (default) | Sets `ConditionDegraded=True` with `Reason=ImageVerificationFailed` and blocks StatefulSet updates |
| `Warn` | Logs an error and emits a Kubernetes Event but proceeds with StatefulSet updates |

## Reconciliation Semantics

- Watches `OpenBaoCluster` and all owned resources (StatefulSet, Services, ConfigMaps, Secrets, ServiceAccounts, Ingresses, NetworkPolicies).
- Ensures the rendered `config.hcl` and StatefulSet template are consistent with Spec (including multi-tenant naming conventions).
- Performs image signature verification before StatefulSet creation/updates when `spec.imageVerification.enabled` is `true`.
- Automatically creates NetworkPolicies to enforce cluster isolation.
- Uses controller-runtime patch semantics to apply only necessary changes.
- Updates `Available`/`Degraded` conditions based on StatefulSet readiness.
- **OwnerReferences:** All created resources have `OwnerReferences` pointing to the parent `OpenBaoCluster`, enabling automatic garbage collection when the cluster is deleted.

## Configuration Generation

### The `config.hcl` Template

The Operator generates the OpenBao configuration file dynamically:

```hcl
ui = true

listener "tcp" {
  address = "0.0.0.0:8200"
  cluster_address = "0.0.0.0:8201"
  # Operator injects paths to the Secret it manages
  tls_cert_file = "/etc/bao/tls/tls.crt"
  tls_key_file  = "/etc/bao/tls/tls.key"
  tls_client_ca_file = "/etc/bao/tls/ca.crt"
}

seal "static" {
  current_key    = "file:///etc/bao/unseal/key"
  current_key_id = "operator-generated-v1"
}

storage "raft" {
  path = "/bao/data"

  # Bootstrap retry_join targeting pod-0
  retry_join {
    leader_api_addr = "https://prod-cluster-0.prod-cluster.security.svc:8200"
    leader_ca_cert_file = "/etc/bao/tls/ca.crt"
    leader_client_cert_file = "/etc/bao/tls/tls.crt"
    leader_client_key_file = "/etc/bao/tls/tls.key"
  }

  # Post-initialization dynamic join
  retry_join {
    auto_join               = "provider=k8s namespace=security label_selector=\"openbao.org/cluster=prod-cluster\""
    leader_ca_cert_file     = "/etc/bao/tls/ca.crt"
    leader_client_cert_file = "/etc/bao/tls/tls.crt"
    leader_client_key_file  = "/etc/bao/tls/tls.key"
  }
}
```

!!! note "Hybrid Join Approach"
    We use a hybrid approach. A deterministic bootstrap `retry_join` to pod-0 avoids split-brain during Day 0, while the post-initialization `auto_join` with the Kubernetes provider (`provider=k8s`) enables dynamic, label-driven scaling without editing the ConfigMap.

### Configuration Ownership and Merging

- The Operator owns the critical `listener "tcp"` and `storage "raft"` stanzas to guarantee mTLS and Raft stability.
- Users may provide additional non-protected configuration settings via `spec.config` as key/value pairs, but attempts to override protected stanzas are rejected by validation enforced at admission time:
  - CRD-level CEL rules block obvious misuse (for example, top-level `listener`, `storage`, `seal` keys).
  - A `ValidatingAdmissionPolicy` performs deeper, semantic checks and enforces additional constraints on `spec.config` keys and values.
- The ConfigController merges user attributes into an operator-generated template in a deterministic, idempotent way, skipping any operator-owned keys.
