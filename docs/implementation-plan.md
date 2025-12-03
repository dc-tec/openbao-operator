# Implementation Plan: OpenBao Supervisor Operator

This document translates the High-Level Design (HLD), Technical Design Document (TDD), Functional Requirements, and Threat Model into a concrete, step-by-step implementation plan for the OpenBao Supervisor Operator.

The plan is ordered to de-risk fundamentals first (API, PKI, static auto-unseal, basic cluster formation), then layer on upgrades, backups, observability, and multi-tenancy hardening.

-----

## 0. Foundations and Project Bootstrap

**Goals**

- Scaffold a Kubebuilder-based Operator project aligned with the API and controller structure described in the TDD.
- Establish basic tooling (linting, testing, CI) before significant code is written.

**Tasks**

- Initialize the project:
  - `kubebuilder init --domain openbao.org --repo github.com/openbao/operator`
  - Configure Go module path, Go version, and controller-runtime version.
- Create the primary API group and version:
  - `kubebuilder create api --group openbao --version v1alpha1 --kind OpenBaoCluster --resource --controller`
- Set up basic tooling:
  - Add `make` targets for `test`, `lint`, `manifests`, `generate`.
  - Integrate a linter (e.g., `golangci-lint`) and ensure it runs locally and in CI.

Acceptance criteria:

- Repository builds and runs `make test` / `go test ./...` successfully.
- Generated CRD and controller scaffolding compile with no lints at the default level.

-----

## 1. API Definition and Validation

**Goals**

- Implement the `OpenBaoCluster` API exactly as specified in the TDD, including Spec, Status, `DeletionPolicy`, and `Paused`.
- Enforce basic validation and protected configuration rules.

**Tasks**

- In `api/v1alpha1/openbaocluster_types.go`:
  - Define `OpenBaoClusterSpec` with fields:
    - `Version string`
    - `Image string`
    - `Replicas int32`
    - `Paused bool`
    - `TLS TLSConfig`
    - `Storage StorageConfig`
    - `Service *ServiceConfig`
    - `Ingress *IngressConfig`
    - `Config map[string]string`
    - `Backup *BackupSchedule`
    - `DeletionPolicy DeletionPolicy`
  - Define `TLSConfig`, `StorageConfig`, `BackupSchedule`, and `DeletionPolicy` as per the TDD.
  - Define `OpenBaoClusterStatus` with:
    - `Phase ClusterPhase`
    - `ActiveLeader string`
    - `ReadyReplicas int32`
    - `CurrentVersion string`
    - `LastBackupTime *metav1.Time`
    - `Conditions []metav1.Condition`
- Add CRD markers:
  - `+kubebuilder:subresource:status`
  - Validation markers for required fields, enums for `DeletionPolicy`, and sensible defaults (e.g., `Replicas=3`).
- Protected configuration:
  - Add CRD-level validation for `spec.config` keys as far as possible (e.g., forbid known protected stanza keys).
  - Plan for a Validating Webhook (can be deferred to a later phase) to enforce the full protected-stanza rules.

Acceptance criteria:

- CRD manifests (`make manifests`) reflect the Spec/Status and validation rules.
- `kubectl apply` of a minimal `OpenBaoCluster` passes schema validation.

-----

## 2. Core Controller Wiring and `spec.paused` Semantics

**Goals**

- Implement a single `OpenBaoCluster` controller that orchestrates the sub-responsibilities (Cert, Infrastructure, Upgrade, Backup) initially.
- Ensure `spec.paused` is honored consistently across all reconciliation logic.

**Tasks**

- In `controllers/openbaocluster_controller.go`:
  - Implement `Reconcile` skeleton:
    - Fetch `OpenBaoCluster`.
    - If `Spec.Paused == true`, log and short-circuit reconciliation (except for delete/finalizers).
    - Dispatch to internal helpers: `reconcileCerts`, `reconcileInfra`, `reconcileUpgrade`, `reconcileBackup`.
  - Ensure Status is updated with `Phase` and Conditions (e.g., `Available`, `Degraded`, `Upgrading`, `BackingUp`).
- Register the controller with the manager:
  - Set reasonable concurrency and rate-limiting options in `SetupWithManager`.

Acceptance criteria:

- Pausing a cluster via `spec.paused=true` stops changes to underlying resources while still allowing deletion/finalizers to proceed.

-----

## 3. CertManager: TLS Lifecycle

**Goals**

- Implement PKI lifecycle as per the TDD (CA + server certs, rotation, SIGHUP reload).

**Tasks**

- Implement `reconcileCerts`:
  - Manage a CA Secret (`<cluster>-tls-ca`) with a long-lived Root CA (2048-bit RSA).
  - Manage a server certificate Secret (`<cluster>-tls-server`) with SANs:
    - Internal service names (`*.openbao.svc`, `<cluster>-0.openbao.svc`, etc.).
    - Any `TLS.ExtraSANs`.
    - `127.0.0.1` and `localhost`.
  - Implement rotation:
    - Parse certificate NotAfter.
    - If expiry window < configured rotation period (default 7 days), issue new certs.
    - Update Secrets atomically.
  - SIGHUP:
    - Compute a hash of the current server cert.
    - Annotate pods/StatefulSet with the hash.
    - When the hash changes, use `pods/exec` to send `SIGHUP` to the OpenBao process in each pod.
- Update Status:
  - Maintain a `TLSReady` Condition with detailed reasons on failure.

Tests (envtest):

- CA and server Secrets are created for a new cluster.
- Rotation occurs when the certificate is near expiry.
- Hash changes trigger SIGHUP (simulated via mock exec).

-----

## 4. InfrastructureManager: Static Auto-Unseal, Config, StatefulSet, and Services

**Goals**

- Implement ConfigMap generation (including static auto-unseal), StatefulSet reconciliation, internal Service, and optional external access.

**Tasks**

- Static auto-unseal:
  - Implement helper in `reconcileInfra`:
    - Look for per-cluster unseal Secret (`<cluster>-unseal-key` or similar).
    - If absent, generate 32 random bytes (cryptographically secure), base64-encode, and store as a Secret.
  - Ensure the StatefulSet mounts this Secret at `/etc/bao/unseal/key`.
  - Inject `seal "static"` stanza into the generated `config.hcl` template:
    - `current_key = "file:///etc/bao/unseal/key"`
    - `current_key_id = "operator-generated-v1"`.
- ConfigMap generation:
  - Build `config.hcl` from:
    - Operator-owned `listener "tcp"` and `storage "raft"` stanzas (with `retry_join`).
    - TLS file paths (`/etc/bao/tls/...`).
    - The static `seal "static"` stanza.
    - Merge in user `spec.config` fragments for non-protected keys only.
- StatefulSet:
  - Ensure desired replicas, image, volume mounts (TLS, unseal, data), and readiness/liveness probes.
  - Configure updateStrategy to enable partition-based upgrades (used later by UpgradeManager).
- Services and external access:
  - Internal headless Service for the StatefulSet.
  - Optional external Service/Ingress as per `spec` (if defined).
  - Ensure external hostnames are added to TLS SANs (via `TLS.ExtraSANs` or spec-derived).
- DeletionPolicy and finalizers:
  - Add a finalizer to `OpenBaoCluster`.
  - On delete:
    - Apply `DeletionPolicy` to StatefulSet and PVCs.
    - Leave or attempt to delete external backups as specified.

Tests (envtest/kind where needed):

- ConfigMap and StatefulSet are created correctly for a new cluster.
- The unseal Secret is created once and reused.
- DeletionPolicy is honored (PVCs retained/deleted as configured).

-----

## 5. UpgradeManager: Raft-Aware Rolling Updates

**Goals**

- Implement the upgrade state machine using StatefulSet partitions, leader detection, and safe step-down.

**Tasks**

- Health and leader discovery:
  - Implement a small OpenBao client wrapper that can:
    - Query `/v1/sys/health` for seal/health status.
    - Identify the current Raft leader.
- Upgrade logic in `reconcileUpgrade`:
  - Detect drift between `Spec.Version` and `Status.CurrentVersion`.
  - Set StatefulSet `updateStrategy.rollingUpdate.partition` to `replicas` to pause updates.
  - Iterate pods in reverse ordinal:
    - If target pod is leader, call `PUT /v1/sys/step-down` and wait for new leader.
    - Decrement partition to allow update of target pod.
    - Wait for Ready and health checks to pass (unsealed, initialized).
  - Update `Status.CurrentVersion` and Conditions (`Upgrading`, `Degraded`) as progress is made.
- Pause integration:
  - Ensure upgrades do not proceed when `spec.paused=true`.

Tests (envtest + mocked OpenBao client, plus E2E):

- Upgrades skip/step down the leader correctly.
- Upgrade can resume correctly after operator restart (state persisted in Status).

-----

## 6. BackupManager: Snapshots to Object Storage

**Goals**

- Implement scheduled backups using OpenBao’s Raft snapshot API and streaming to generic object storage.

**Tasks**

- Backup configuration:
  - Parse `BackupSchedule` from Spec (Cron expression, endpoint, bucket, path prefix).
  - Validate backup schedules to avoid excessively frequent backups that could cause resource exhaustion (e.g., reject or warn on intervals below an operator-configurable minimum).
  - Use an in-operator scheduler (or a minimal Cron-like reconciler pattern) to trigger backups.
- Snapshot streaming:
  - Discover current leader using the OpenBao client.
  - Call `GET /v1/sys/storage/raft/snapshot` on the leader.
  - Stream body directly to object storage using the cloud-agnostic client (e.g., interface over S3/GCS/Azure-like SDKs), with no disk buffering.
  - Name objects predictably using timestamp and cluster name.
- Status and Conditions:
  - Update `Status.LastBackupTime` on success.
  - Increment backup failure metrics and set `BackingUp`/`Degraded` Conditions on failure.

Tests:

- Use MinIO in E2E/CI to validate streaming behavior and object naming.
- Verify Status updates and metrics for success/failure.

-----

## 7. Observability: Metrics and Logging

**Goals**

- Expose the metrics and structured logs defined in the TDD and FRs.

**Tasks**

- Metrics:
  - Implement Prometheus metrics:
    - `openbao_cluster_ready_replicas`
    - `openbao_upgrade_status`
    - `openbao_backup_last_success_timestamp`
    - `openbao_backup_failure_total`
    - `openbao_reconcile_duration_seconds`
    - `openbao_reconcile_errors_total`
  - Ensure metrics are labeled with `namespace`, `name`, and controller where appropriate.
- Logging:
  - Standardize structured logging fields:
    - `cluster_namespace`, `cluster_name`, `controller`, `reconcile_id`.
  - Avoid logging Secrets or unseal key material.

- Optional Prometheus Operator integration:
  - Provide example `ServiceMonitor`/`PodMonitor` manifests (or Kustomize overlays) that scrape the Operator's metrics Service, for users running the Prometheus Operator.

Tests:

- Unit tests to ensure metrics are updated in expected scenarios (e.g., ready replicas change, backup failure).

-----

## 8. Multi-Tenancy Hardening and RBAC

**Goals**

- Ensure safe multi-tenant behavior and RBAC as described in the docs.

**Tasks**

- Resource scoping:
  - Verify all created resources (Secrets, ConfigMaps, StatefulSets, Services) are namespaced and uniquely named per `OpenBaoCluster`.
  - Avoid any global/shared resources across clusters.
- RBAC:
  - Define `ClusterRole`/`Role` and `ClusterRoleBinding`/`RoleBinding` manifests to:
    - Allow the operator access to Secrets, StatefulSets, Pods, `pods/exec`, ConfigMaps in relevant namespaces.
    - Allow namespace-scoped teams to manage only their `OpenBaoCluster` CRs.
- Rate limiting:
  - Configure controller-runtime rate limiting and per-controller `MaxConcurrentReconciles` to prevent hot loops from overwhelming the API server in multi-tenant environments, while still supporting at least 10 concurrent `OpenBaoCluster` resources under normal load.

Tests:

- envtest or unit tests for namespacing and naming conventions.
- Validation that multiple `OpenBaoCluster` objects in different namespaces reconcile independently.

-----

## 9. Webhooks and Advanced Validation (Optional but Recommended)

**Goals**

- Enforce protected configuration rules and immutability constraints beyond what the CRD schema can express.

**Tasks**

- Implement a Validating Webhook for `OpenBaoCluster`:
  - Reject changes that attempt to:
    - Modify protected stanzas (`listener "tcp"`, `storage "raft"`) via `spec.config`.
    - Change immutable fields after creation (e.g., storage size, some TLS policies) where appropriate.
    - Configure obviously unsafe backup targets (e.g., endpoints or buckets outside an operator-defined allowlist), where such policy is required.
- Wire webhook into manager and manifests (certs, service).

Tests:

- Webhook unit tests for allowed vs. rejected configurations.

-----

## 10. Testing & CI Integration

**Goals**

- Automate the testing strategy defined in the TDD.

**Tasks**

- Unit tests:
  - Ensure coverage of all major helper functions (config generation, static auto-unseal key management, metrics updates).
- envtest integration tests:
  - Full reconcile flow for a single `OpenBaoCluster` (create → ready).
  - Pause/resume semantics.
  - DeletionPolicy behavior.
- E2E tests (kind + KUTTL or Ginkgo):
  - Scenarios:
    - Create cluster → verify pods Ready and OpenBao Raft peers healthy.
    - Upgrade image → verify controlled, leader-aware upgrades.
    - Backup schedule → verify snapshot object appears in MinIO.
    - Multi-tenant scenario with multiple clusters and no cross-impact.
    - Basic load test with at least 10 `OpenBaoCluster` resources to validate reconcile latency (targeting ≤ 30 seconds under normal load) and absence of hot-loop behavior.
- Define and maintain a test matrix:
  - Target at least the following Kubernetes versions in CI: `v1.28`, `v1.29`, `v1.30`, `v1.31` (subject to availability in kind).
  - Target at least two OpenBao versions (e.g., `2.1.x` and the latest stable minor) to validate upgrade paths.
- CI configuration:
  - Run unit + envtest on each PR.
  - Gate merges on E2E suite at least on main branch or nightly.

Acceptance criteria:

- CI reliably runs and gates merges on the agreed test suite.

-----

## 11. Documentation and Operational Guides

**Goals**

- Provide user-facing documentation aligned with the implemented behavior.

**Tasks**

- Author user docs:
  - Example `OpenBaoCluster` manifests (minimal, external access, with backups).
  - Explanation of `Paused`, `DeletionPolicy`, and static auto-unseal behavior.
  - Recommended RBAC and multi-tenancy patterns.
  - Ingress and service mesh integration patterns, including example configurations that preserve end-to-end mTLS.
  - Backup configuration and security best practices (e.g., recommended schedules, per-tenant buckets/prefixes, least-privilege credentials).
  - A compatibility and support matrix covering minimum supported Kubernetes and OpenBao versions, and how unsupported versions are surfaced via `Status.Conditions`.
- Operator runbook:
  - Upgrade procedures.
  - Backup and restore (manual) procedures.
  - Common failure modes and remediation steps.

Acceptance criteria:

- Documentation is consistent with HLD/TDD and the actual code behavior.

-----

This plan is meant to be iterative: early phases (0–4) should be implemented and validated first to achieve a minimal, secure, TLS + static-auto-unseal OpenBao cluster. Subsequent phases (5–11) can then be layered on, with frequent E2E validation to ensure regressions are caught early.
