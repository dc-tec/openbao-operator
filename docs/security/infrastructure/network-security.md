# Network Security

The Operator adopts a "Default Deny" network posture for every OpenBao cluster it creates.

## Automated NetworkPolicies

For every `OpenBaoCluster`, the operator automatically creates a Kubernetes `NetworkPolicy`.

- **Default Ingress Deny:** All ingress traffic is blocked by default.
- **Allow Rules:**
  - **Inter-Pod Traffic:** Allows traffic between OpenBao pods within the same cluster (required for Raft replication).
  - **Operator Access:** Allows ingress from the OpenBao Operator pods on port 8200 (required for upgrade operations such as leader step-down; initialization prefers Kubernetes service-registration signals but the operator may still need API access for non-self-init initialization and as a fallback).
  - **Kube-System:** Allows ingress from `kube-system` for necessary components like DNS.
  - **Gateway API:** If `spec.gateway` is enabled, traffic is allowed from the Gateway's namespace.

## Egress Control

Egress is restricted to essential services:

- **DNS:** UDP/TCP on port 53.
- **Kubernetes API:** TCP on port 443/6443 (required for the `discover-k8s` provider to find peer pods).
- **Kubernetes API (Service Registration):** OpenBao uses Kubernetes service registration to update Pod labels (leader/initialized/sealed/version), which requires in-cluster API access from the OpenBao pods.
- **Cluster Peers:** Communication with other Raft peers.

!!! note "Backup Jobs"
    Backup job pods run in separate pods that are excluded from this strict policy to allow them to reach external object storage endpoints (S3, GCS, etc.).

## Custom Network Rules

While the operator enforces a default deny posture, users can extend the NetworkPolicy with custom ingress and egress rules via `spec.network.ingressRules` and `spec.network.egressRules`. These custom rules are merged with the operator-managed rules, ensuring that essential operator rules (DNS, API server, cluster peers) are always present and cannot be overridden.

**Security Considerations:**

- Custom rules should follow the principle of least privilege
- Only allow access to specific namespaces, IPs, or ports as needed
- Review custom rules regularly to ensure they remain necessary
- For transit seal backends, prefer namespace selectors over broad IP ranges
- Consider using backup jobs (excluded from NetworkPolicy) rather than adding broad egress rules for object storage access

See also: [Network Configuration](../../user-guide/network.md)

## PodDisruptionBudget

The operator automatically creates a `PodDisruptionBudget` (PDB) for every `OpenBaoCluster`:

- **Configuration:** `maxUnavailable: 1` ensures at least N-1 pods remain available during voluntary disruptions.
- **Purpose:** Prevents simultaneous pod evictions during node drains, cluster upgrades, or autoscaler actions that could cause Raft quorum loss.
- **Automatic Creation:** PDB is created alongside the StatefulSet and owned by the `OpenBaoCluster` for garbage collection.
- **Single-Replica Exclusion:** PDB is skipped for single-replica clusters where it provides no benefit.

## Sentinel Rate Limiting

To prevent Sentinel-triggered reconciliations from indefinitely blocking administrative operations (backups, upgrades), rate limiting is enforced:

- **Consecutive Fast Path Limit:** After 5 consecutive Sentinel-triggered reconciliations, a full reconcile (including Upgrade and Backup managers) is forced.
- **Time-Based Limit:** If 5 minutes have elapsed since the last full reconcile, the next reconcile runs all managers regardless of Sentinel trigger status.
- **Tracking Fields:** `status.drift.lastFullReconcileTime` and `status.drift.consecutiveFastPaths` track reconciliation patterns.
- **Security Purpose:** Prevents a compromised or misbehaving Sentinel from causing a denial-of-service of administrative functions.

## Job Resource Limits

Backup and restore Jobs have resource limits to prevent resource exhaustion:

| Resource | Request | Limit |
|----------|---------|-------|
| CPU      | 100m    | 500m  |
| Memory   | 128Mi   | 512Mi |

This prevents runaway jobs from consuming excessive node resources and impacting other workloads.
