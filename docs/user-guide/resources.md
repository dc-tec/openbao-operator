# Managed Resources

## Prerequisites

- **OpenBaoCluster**: An active `OpenBaoCluster` CR.

## Resources List

For a cluster named `dev-cluster` in namespace `security`, the Operator reconciles:

- TLS Secrets (when `spec.tls.mode` is `OperatorManaged` or omitted):
  - `dev-cluster-tls-ca` – Root CA (`ca.crt`, `ca.key`). Created and managed by the operator.
  - `dev-cluster-tls-server` – server cert/key and CA bundle (`tls.crt`, `tls.key`, `ca.crt`). Created and managed by the operator.
- TLS Secrets (when `spec.tls.mode` is `External`):
  - `<cluster-name>-tls-ca` - CA certificate Secret
  - `<cluster-name>-tls-server` - Server certificate Secret
- No TLS Secrets (when `spec.tls.mode` is `ACME`):
  - OpenBao manages certificates internally via ACME client
  - `dev-cluster-tls-ca` – CA certificate (`ca.crt`, optionally `ca.key`). Must be created by an external provider (e.g., cert-manager).
  - `dev-cluster-tls-server` – server cert/key and CA bundle (`tls.crt`, `tls.key`, `ca.crt`). Must be created by an external provider. The operator waits for these Secrets and triggers hot-reloads when they change.
- Auto-unseal Secret (only when using static seal):
  - `dev-cluster-unseal-key` – raw 32-byte unseal key (created only if `spec.unseal` is omitted or `spec.unseal.type` is `"static"`).
- Root token Secret (created during initialization):
  - `dev-cluster-root-token` – initial root token (`token` key). See Security section below.
- ConfigMap:
  - `dev-cluster-config` – rendered `config.hcl` containing:
    - `listener "tcp"` with:
      - TLS file paths under `/etc/bao/tls` (when `spec.tls.mode` is `OperatorManaged` or `External`)
      - ACME parameters (`tls_acme_ca_directory`, `tls_acme_domains`, etc.) when `spec.tls.mode` is `ACME`
    - `seal` stanza (type depends on `spec.unseal.type`):
      - `seal "static"` pointing at `/etc/bao/unseal/key` (default).
      - `seal "awskms"`, `seal "gcpckms"`, `seal "azurekeyvault"`, `seal "transit"`, `seal "kmip"`, `seal "ocikms"`, or `seal "pkcs11"` with structured provider-specific configuration (when `spec.unseal.type` is set and the corresponding seal config is provided, e.g., `spec.unseal.transit`, `spec.unseal.awskms`, etc.).
    - `storage "raft"` with `path = "/bao/data"` and:
      - a bootstrap `retry_join` targeting pod-0 for initial cluster formation.
      - a post-initialization `retry_join` using `auto_join = "provider=k8s namespace=<ns> label_selector=\"openbao.org/cluster=<name>\""` so additional pods can join dynamically.
      - TLS certificate file paths in `retry_join` (when `spec.tls.mode` is `OperatorManaged` or `External`)
      - No TLS file paths in `retry_join` when `spec.tls.mode` is `ACME` (OpenBao handles cluster TLS automatically)
    - `service_registration "kubernetes"` (so OpenBao can update Pod labels like `openbao-active`, `openbao-initialized`, and `openbao-sealed`)
    - Structured configuration from `spec.configuration` (UI, logging, plugin, lease/TTL, cache, advanced features, listener, raft, ACME CA root).
    - Audit device blocks from `spec.audit` (if configured).
    - Plugin blocks from `spec.plugins` (if configured).
    - Telemetry block from `spec.telemetry` (if configured).
- NetworkPolicy:
  - `dev-cluster-network-policy` – automatically created to enforce cluster isolation:
    - Default deny all ingress traffic.
    - Allow ingress from pods within the same cluster (same pod selector labels).
    - Allow ingress from kube-system namespace (for DNS and system components).
    - Allow ingress from OpenBao operator pods on port 8200 (for upgrade operations such as leader step-down; initialization prefers Kubernetes service-registration signals but the operator may still need API access for non-self-init initialization and as a fallback).
    - Allow ingress from Gateway namespace (if Gateway API is enabled).
    - Allow egress to DNS (port 53 UDP/TCP) for service discovery.
    - Allow egress to Kubernetes API server (port 443) for pod discovery (discover-k8s provider). The operator auto-detects the API server CIDR by querying the `kubernetes` Service in the `default` namespace. If auto-detection fails (e.g., due to restricted permissions in multi-tenant environments), users can manually configure `spec.network.apiServerCIDR` as a fallback.
    - Allow egress to cluster pods on ports 8200 and 8201 for Raft communication.
    - **Custom Rules:** Users can specify additional ingress and egress rules via `spec.network.ingressRules` and `spec.network.egressRules`. These are merged with the operator-managed rules (operator rules cannot be overridden). See [Network Configuration](network.md) for details.
    - **Note:** Backup job pods (labeled with `openbao.org/component=backup`) are excluded from this NetworkPolicy to allow access to object storage. Users can create custom NetworkPolicies for backup jobs if additional restrictions are needed.
- Services:
  - Headless Service `dev-cluster` (ClusterIP `None`) backing the StatefulSet.
  - Optional external Service `dev-cluster-public` when `spec.service` or `spec.ingress.enabled` is set.
- StatefulSet:
  - `dev-cluster` with:
    - `replicas = spec.replicas`.
    - PVC template `data` sized from `spec.storage.size`.
    - Volumes:
      - TLS Secret at `/etc/bao/tls`.
      - ConfigMap at `/etc/bao/config`.
      - Unseal Secret at `/etc/bao/unseal/key` (only when using static seal).
      - KMS credentials at `/etc/bao/seal-creds` (when using external KMS with `spec.unseal.credentialsSecretRef`).
      - Data at `/bao/data`.
    - Probes:
      - Liveness treats `sealed`/`uninitialized` as success to avoid restart loops during bootstrap.
      - Readiness requires initialized and unsealed (standby ok).
      - StartupProbe allows extra time for initial Raft/bootstrap before liveness enforcement.

You can inspect these resources with:

```sh
kubectl -n security get statefulsets,svc,cm,secrets -l openbao.org/cluster=dev-cluster
```
