# InfrastructureManager (Config & StatefulSet)

**Responsibility:** Render configuration and maintain the StatefulSet.

## Configuration Strategy

- We do not use a static ConfigMap. We generate it dynamically from `OpenBaoCluster.spec` and cluster topology (service name/namespace/ports).
- **Injection:** We explicitly inject the `listener "tcp"` block:
  - For `OperatorManaged` and `External` modes: Points to mounted TLS Secrets at `/etc/bao/tls`.
  - For `ACME` mode: Includes ACME parameters (`tls_acme_ca_directory`, `tls_acme_domains`, etc.) and no file paths.

## Discovery Strategy

- The operator uses Kubernetes go-discover based `retry_join` with `auto_join = "provider=k8s namespace=<ns> label_selector=\"openbao.org/cluster=<cluster-name>\""` so pods can join dynamically based on labels rather than a static peer list.
- For blue/green upgrades, the operator can narrow the `label_selector` to include the active revision label so that joining pods discover the correct peer set.
- `leader_tls_servername` is set to a stable per-cluster name (`openbao-cluster-<cluster-name>.local`) to support mTLS verification.

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
- When verification succeeds, the operator pins `spec.image` to the verified digest and (when configured) also pins `spec.initContainer.image` to the verified digest to prevent TOCTOU attacks.

**Failure Policy:**

| Policy | Behavior |
| ------ | -------- |
| `Block` (default) | Blocks reconciliation of the StatefulSet and records an image verification failure condition |
| `Warn` | Logs an error and emits a Kubernetes Event but proceeds with StatefulSet updates |

## Reconciliation Semantics

- Watches `OpenBaoCluster` only (no child `Owns()` watches) due to the least-privilege RBAC model.
- Ensures the rendered `config.hcl` and StatefulSet template are consistent with Spec (including multi-tenant naming conventions).
- Performs image signature verification before StatefulSet creation/updates when `spec.imageVerification.enabled` is `true`.
- Automatically creates NetworkPolicies to enforce cluster isolation.
- Uses controller-runtime patch semantics to apply only necessary changes.
- Surfaces readiness/errors via the workload status subtree; `status.conditions` is aggregated by the status controller.
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
  node_id = "${HOSTNAME}"

  retry_join {
    auto_join               = "provider=k8s namespace=security label_selector=\"openbao.org/cluster=prod-cluster\""
    leader_tls_servername   = "openbao-cluster-prod-cluster.local"
    leader_ca_cert_file     = "/etc/bao/tls/ca.crt"
    leader_client_cert_file = "/etc/bao/tls/tls.crt"
    leader_client_key_file  = "/etc/bao/tls/tls.key"
  }
}

service_registration "kubernetes" {}
```

### Configuration Ownership and Merging

- The Operator owns the critical `listener "tcp"` and `storage "raft"` stanzas to guarantee mTLS and Raft stability.
- User-tunable configuration is expressed via typed fields:
  - `spec.configuration` (logging, cache flags, plugin behavior, etc.)
  - `spec.audit`, `spec.plugins`, `spec.telemetry` (rendered into dedicated HCL stanzas)
- Operator-owned fields are rendered deterministically and are not user-overridable (for example: listener address/cluster_address, raft path, retry_join, and seal when using static unseal).
