# Architecture: OpenBao Supervisor Operator

This document provides a comprehensive overview of the OpenBao Operator's architecture, internal design, and implementation details. It merges the high-level design and technical design documentation into a single reference for contributors.

## 1. Architecture Overview

The OpenBao Operator acts as an autonomous administrator for OpenBao clusters on Kubernetes. Unlike legacy operators that attempt to micro-manage internal consensus (Raft), this Operator adopts a **Supervisor Pattern**. It delegates data consistency to the OpenBao binary (via `retry_join` and static auto-unseal) while managing the external ecosystem: PKI lifecycle, Infrastructure state, and Safe Version Upgrades.

The design is intentionally **cloud-agnostic** and **multi-tenant**:

- **Cloud-agnostic**: The Operator relies only on standard Kubernetes APIs and generic HTTP/S-compatible object storage, without hard-coding provider-specific features.
- **Multi-tenant**: A single Operator instance manages multiple `OpenBaoCluster` resources across namespaces, while ensuring isolation and independence between clusters.

### 1.1 System Components

At a high level, the architecture consists of:

- **Operator Controller Manager:** Runs multiple reconcilers (Cert, Config, StatefulSet, Upgrade, Backup) in a single process.
- **OpenBao StatefulSets:** One StatefulSet per `OpenBaoCluster` providing Raft storage and HA.
- **Sentinel Sidecar Controller:** An optional per-cluster Deployment that watches for infrastructure drift and triggers fast-path reconciliation. When enabled, the Sentinel detects unauthorized changes to managed resources (StatefulSets, Services, ConfigMaps, Secrets) and immediately triggers the operator to correct them, bypassing expensive operations like upgrades and backups.
- **Persistent Volumes:** Provided by the cluster's StorageClass for durable Raft data.
- **Object Storage:** Generic HTTP/S-compatible object storage for snapshots (S3/GCS/Azure Blob or compatible endpoints).
- **Users / Tenants:** Developers and platform teams who create and manage `OpenBaoCluster` resources in their own namespaces.
- **Static Auto-Unseal Key:** A per-cluster unseal key managed by the Operator and stored in a Kubernetes Secret, used by OpenBao's static auto-unseal mechanism.
- **RBAC Architecture (Least-Privilege Model):**
  - **Provisioner ServiceAccount (`openbao-operator-provisioner`):** Has minimal cluster-wide permissions to manage `OpenBaoTenant` CRDs and create Roles/RoleBindings in tenant namespaces. Cannot access workload resources directly. Does not have `list` or `watch` permissions on namespaces, preventing cluster topology enumeration.
  - **OpenBaoCluster Controller ServiceAccount (`openbao-operator-controller`):** Has cluster-wide read access to `openbaoclusters` (`get;list;watch`) and cluster-wide `create` on `tokenreviews`/`subjectaccessreviews` (for the protected metrics endpoint). All OpenBaoCluster writes and all child resources are tenant-scoped via namespace Roles created by the Provisioner.
  - **Namespace Provisioner Controller:** Watches `OpenBaoTenant` CRDs that explicitly declare target namespaces for provisioning. Creates namespace-scoped Roles and RoleBindings that grant the OpenBaoCluster Controller ServiceAccount the permissions needed to manage OpenBaoCluster resources in those namespaces.

The Operator treats `OpenBaoCluster.Spec` as the declarative source of truth and continuously reconciles Kubernetes resources and OpenBao state to match this desired state.

### 1.2 Component Interaction

1. **User:** Submits `OpenBaoCluster` Custom Resource (CR).
2. **CertController:** Observes CR, generates CA and Leaf Certs into Kubernetes Secrets.
3. **ConfigController:** Generates the `config.hcl` (ConfigMap) injecting TLS paths and `retry_join` stanzas.
4. **StatefulSetController:** Ensures the StatefulSet exists, matches the desired version, and mounts the Secrets/ConfigMaps.
5. **UpgradeController:** Intercepts version changes to perform Raft-aware rolling updates.
6. **Sentinel (if enabled):** Watches for infrastructure drift and triggers fast-path reconciliation when unauthorized changes are detected.
7. **OpenBao Pods:** Boot up, mount certs, read config, and auto-join the cluster using the K8s API peer discovery.

### 1.3 Assumptions and Non-Goals

**Assumptions:**

- The Kubernetes cluster provides:
  - A default StorageClass for persistent volumes.
  - Working DNS for StatefulSet pod and service names.
  - A mechanism to provide credentials for object storage (e.g., Secrets, workload identity).
- **OpenBao Version:** The Operator uses static auto-unseal, which requires **OpenBao v2.4.0 or later**. Versions below 2.4.0 do not support the `seal "static"` configuration and will fail to start.
- Platform teams are responsible for:
  - Deploying and upgrading the Operator itself (or enabling an optional self-managed mode).
  - Wiring the Operator and OpenBao telemetry into their observability stack.

**Non-Goals (v0.1):**

- Cross-cluster or multi-region OpenBao federation.
- Automated disaster **restore** flows (restore may be manual for v0.1).
- Deep integration with provider-specific IAM mechanisms (e.g., IRSA); these can be layered on by platform teams.
- Provider-specific integrations (e.g., native AWS/GCP/Azure APIs); these can be added as optional extensions.

## Cross-Cutting Concerns

### Error Handling & Rate Limiting

All Managers (Cert, Infrastructure, Upgrade, Backup) share the following error-handling and reconciliation semantics:

- External operations (Kubernetes API, OpenBao API, object storage SDKs) MUST:
  - Accept `context.Context` as the first parameter.
  - Use time-bounded contexts so that calls respect reconciliation deadlines and cancellation.
- Transient errors (e.g., network timeouts, optimistic concurrency conflicts) are retried via the controller-runtime workqueue rate limiter rather than custom `time.Sleep` loops.
- Per-controller `MaxConcurrentReconciles` values are configured to:
  - Prevent a single noisy `OpenBaoCluster` from starving others.
  - Support at least 10 concurrent `OpenBaoCluster` resources under normal load.
- Persistent failures MUST:
  - Surface as typed `Conditions` (e.g., `ConditionDegraded`, `ConditionUpgrading`, `ConditionBackingUp`) with clear reasons and messages.
  - Avoid hot reconcile loops by relying on exponential backoff from the rate limiter.
- Audit logging:
  - Critical operations (initialization, leader step-down) MUST emit structured audit logs tagged with `audit=true` and `event_type` for security monitoring.

### Observability & Metrics

The Operator exposes metrics suitable for Prometheus-style scraping and emits structured audit logs for critical operations.

**Core Metrics:**

- `openbao_cluster_ready_replicas` - Number of Ready replicas
- `openbao_cluster_phase` - Current cluster phase
- `openbao_reconcile_duration_seconds` - Reconciliation duration
- `openbao_reconcile_errors_total` - Reconciliation errors

**Upgrade Metrics:**

- `openbao_upgrade_status` - Upgrade status
- `openbao_upgrade_duration_seconds` - Total upgrade duration
- `openbao_upgrade_stepdown_total` - Total leader step-down operations

**Backup Metrics:**

- `openbao_backup_last_success_timestamp` - Timestamp of last successful backup
- `openbao_backup_success_total` - Total successful backups
- `openbao_backup_failure_total` - Total backup failures

**TLS Metrics:**

- `openbao_tls_cert_expiry_timestamp` - Unix timestamp when the current server certificate expires (per `namespace`, `name`, `type`)
- `openbao_tls_rotation_total` - Total number of server certificate rotations per cluster

**Logging:**

- Structured logs include: `namespace`, `name`, `controller`, `reconcile_id`
- Log levels: Error (failures), Warn (recoverable issues), Info (normal operations), Debug (detailed operations)
- **Sensitive Data:** The following MUST NEVER be logged:
  - Root tokens or any authentication tokens
  - Unseal keys or key material
  - Backup contents or snapshot data
  - Credentials for object storage

## API Specification

The `OpenBaoCluster` Custom Resource Definition defines the desired state and observed status of an OpenBao cluster.

### Spec (Desired State)

Key fields include:

- `version`: Semantic OpenBao version (e.g., "2.1.0")
- `image`: Container image to run
- `replicas`: Number of replicas (default: 3)
- `paused`: If true, reconciliation is paused
- `tls`: TLS configuration (enabled, mode, rotationPeriod)
- `storage`: Storage configuration (size, storageClassName)
- `backup`: Backup schedule and target configuration
- `selfInit`: Self-initialization configuration
- `gateway`: Gateway API configuration
- `deletionPolicy`: Behavior when CR is deleted (Retain, DeletePVCs, DeleteAll)

### Status (Observability)

Key fields include:

- `phase`: High-level summary (Initializing, Running, Upgrading, BackingUp, Failed)
- `activeLeader`: Current Raft Leader pod name
- `readyReplicas`: Number of Ready replicas
- `currentVersion`: Current OpenBao version running
- `initialized`: Whether cluster has been initialized
- `selfInitialized`: Whether cluster was initialized using self-init
- `upgrade`: Upgrade progress state
- `backup`: Backup status
- `conditions`: Kubernetes-style conditions (Available, TLSReady, Upgrading, Degraded, etc.)
