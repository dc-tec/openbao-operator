# Implementation Plan: OpenBao Supervisor Operator

This document translates the High-Level Design (HLD), Technical Design Document (TDD), Functional Requirements, and Threat Model into a concrete, step-by-step implementation plan for the OpenBao Supervisor Operator.

The plan is ordered to de-risk fundamentals first (API, PKI, static auto-unseal, basic cluster formation), then layer on upgrades, backups, observability, and multi-tenancy hardening.

-----

## 0. Foundations and Project Bootstrap

**Status: Implemented**

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

**Status: Implemented**

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
  - Add CRD-level CEL validation for `spec.config` keys as far as possible (for example, forbid known protected stanza keys such as `listener`, `storage`, `seal`).
  - Plan for a Validating Admission Webhook (can be deferred to a later phase) to enforce the full protected-stanza rules using the same config-merge logic as the Operator.

Acceptance criteria:

- CRD manifests (`make manifests`) reflect the Spec/Status and validation rules.
- `kubectl apply` of a minimal `OpenBaoCluster` passes schema validation.

-----

## 2. Core Controller Wiring and `spec.paused` Semantics

**Status: Implemented**

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

**Status: Implemented**

**Goals**

- Implement PKI lifecycle as per the TDD (CA + server certs, rotation, and hot-reload via a sidecar watching TLS changes).

**Tasks**

- Implement `reconcileCerts`:
  - Manage a CA Secret (`<cluster>-tls-ca`) with a long-lived Root CA (2048-bit RSA).
  - Manage a server certificate Secret (`<cluster>-tls-server`) with SANs:
    - Internal service and pod DNS names in the cluster namespace (for example, `*.security.svc`, `<cluster>.security.svc`, `*.<cluster>.security.svc`).
    - Any `TLS.ExtraSANs`.
    - `127.0.0.1` and `localhost`.
  - Implement rotation:
    - Parse certificate NotAfter.
    - If expiry window < configured rotation period (default 7 days), issue new certs.
    - Update Secrets atomically.
  - Hot reload:
    - Compute a hash of the current server cert.
    - Annotate OpenBao pods with the hash (for example, `openbao.org/tls-cert-hash`) to record which certificate is active.
    - When the hash changes, rely on an in-pod sidecar to observe the new hash or updated TLS files and send `SIGHUP` to the OpenBao process locally. The operator itself no longer uses `pods/exec`.
- Update Status:
  - Maintain a `TLSReady` Condition with detailed reasons on failure.

Tests (envtest):

- CA and server Secrets are created for a new cluster.
- Rotation occurs when the certificate is near expiry.
- Hash changes trigger calls to the `ReloadSignaler` (simulated via a recording test double), which is responsible for annotating pods. Sidecar-based tests cover the end-to-end reload behavior.

-----

## 4. InfrastructureManager: Static Auto-Unseal, Config, StatefulSet, and Services

**Status: Implemented**

**Goals**

- Implement ConfigMap generation (including static auto-unseal), StatefulSet reconciliation, internal Service, and optional external access.

**Tasks**

- Static auto-unseal:
  - Implement helper in `reconcileInfra`:
    - Look for per-cluster unseal Secret (`<cluster>-unseal-key`).
    - If absent, generate 32 cryptographically secure random bytes and store as raw bytes in the Secret.
  - Ensure the StatefulSet mounts this Secret at `/etc/bao/unseal/key`.
  - Inject `seal "static"` stanza into the generated `config.hcl` template:
    - `current_key = "file:///etc/bao/unseal/key"`
    - `current_key_id = "operator-generated-v1"`.
- ConfigMap generation:
  - Build `config.hcl` from:
    - Operator-owned `listener "tcp"` and `storage "raft"` stanzas (with `retry_join` / `auto_join`).
    - TLS file paths (`/etc/bao/tls/...`).
    - The static `seal "static"` stanza.
    - Merge in user `spec.config` fragments for non-protected keys only.
- StatefulSet:
  - Ensure desired replicas, image, volume mounts (TLS, unseal, data), and readiness/liveness probes.
  - Configure updateStrategy to enable partition-based upgrades (used later by UpgradeManager).
  - Add config hash annotation to trigger rolling updates on configuration changes.
  - Run as non-root with proper SecurityContext (UID 100, GID 1000).
- Services and external access:
  - Internal headless Service for the StatefulSet.
  - Optional external Service/Ingress as per `spec` (if defined).
  - Ensure external hostnames are added to TLS SANs (via `TLS.ExtraSANs` or spec-derived).
- ServiceAccount:
  - Create a per-cluster ServiceAccount (`<cluster>-serviceaccount`).
- DeletionPolicy and finalizers:
  - Add a finalizer to `OpenBaoCluster`.
  - On delete:
    - Apply `DeletionPolicy` to StatefulSet and PVCs.
    - Leave or attempt to delete external backups as specified.
- Validating Webhook:
  - Implemented defense-in-depth validation for `spec.config` keys.
  - Enforces protected key restrictions and validates key/value syntax.

Tests (envtest/kind where needed):

- ConfigMap and StatefulSet are created correctly for a new cluster.
- The unseal Secret is created once and reused.
- DeletionPolicy is honored (PVCs retained/deleted as configured).

-----

## 4.1. InitManager: Cluster Initialization

**Status: Implemented**

**Goals**

- Automate the initial OpenBao cluster initialization process for new clusters via the HTTP API.
- Ensure single-pod bootstrapping before scaling to desired replicas.
- Securely store the root token for operator access.

**Tasks**

- Initialization workflow:
  - During initial cluster creation, InfraManager starts with 1 replica.
  - InitManager waits for pod-0 container to be running (not necessarily ready, since readiness probe may fail until initialized).
  - Check if OpenBao is initialized via the HTTP health endpoint (`GET /v1/sys/health`) using the per-cluster TLS CA bundle.
  - If not initialized, execute an HTTP initialization request (`PUT /v1/sys/init`) against pod-0 using the in-cluster OpenBao client.
  - Parse the initialization response and store the root token securely.
  - Mark `Status.Initialized = true` to trigger scale-up.
- Root token storage:
  - Store root token in a per-cluster Secret (`<cluster>-root-token`).
  - Token is stored under the `token` key.
  - **Security Note:** This Secret contains highly sensitive credentials and must be protected via RBAC. See Threat Model for details.
- Single-pod bootstrap:
  - InfraManager keeps replicas at 1 until `Status.Initialized` is true.
  - After initialization, InfraManager scales to `Spec.Replicas`.
  - ConfigMap discovery configuration is adjusted based on initialization status:
    - Before initialization: render a single deterministic `retry_join` targeting pod-0.
    - After initialization: render a Kubernetes go-discover based `retry_join` using `auto_join` with the `k8s` provider and the `openbao.org/cluster=<name>` label selector.

Tests:

- Unit tests for initialization status checking.
- envtest coverage for initialization flow and status updates.
- E2E tests verifying cluster bootstraps correctly with single-pod initialization.

-----

## 4.2. Self-Initialization Support

**Status: Implemented**

**Goals**

- Enable opt-in OpenBao self-initialization for declarative cluster bootstrapping.
- Allow users to configure initial audit devices, auth methods, secret engines, and policies via the CRD.
- Ensure the root token is not exposed when self-init is enabled.

**Tasks**

- API changes:
  - Add `SelfInitConfig` struct to `OpenBaoClusterSpec` with `Enabled bool` and `Requests []SelfInitRequest`.
  - Add `SelfInitRequest` struct with `Name`, `Operation`, `Path`, `Data`, and `AllowFailure` fields.
  - Add `SelfInitialized bool` to `OpenBaoClusterStatus`.
  - Add CRD validation for request name regex `^[A-Za-z_][A-Za-z0-9_-]*$`.
- ConfigMap generation:
  - Extend `reconcileInfra` to generate `initialize` stanzas when `spec.selfInit.enabled = true`.
  - Map `spec.selfInit.requests[]` to HCL `initialize` blocks with nested `request` blocks.
  - Ensure proper HCL escaping for complex data structures.
- InitManager changes:
  - When `spec.selfInit.enabled = true`:
    - Skip HTTP-based initialization.
    - Only monitor for `initialized=true` via the HTTP health endpoint.
    - Do NOT create the `<cluster>-root-token` Secret.
    - Set `Status.SelfInitialized = true` after successful initialization.
- Webhook validation:
  - Validate that `spec.selfInit.requests[].name` values are unique.
  - Validate that operation values are valid (`create`, `read`, `update`, `delete`, `list`).

Tests:

- Unit tests for HCL generation with various self-init request configurations.
- envtest coverage for self-init flow (ConfigMap contains initialize stanzas, no root token Secret created).
- E2E tests verifying cluster bootstraps correctly with self-initialization.

-----

## 4.3. Gateway API Support

**Status: Implemented**

**Goals**

- Support Kubernetes Gateway API as an alternative to Ingress for external access.
- Create and manage `HTTPRoute` resources that route traffic through user-managed `Gateway` resources.
- Ensure Gateway hostnames are reflected in TLS SANs.

**Tasks**

- API changes:
  - Add `GatewayConfig` struct to `OpenBaoClusterSpec` with `Enabled`, `GatewayRef`, `Hostname`, `Path`, and `Annotations`.
  - Add `GatewayReference` struct with `Name` and `Namespace` fields.
- RBAC updates:
  - Add permissions for `httproutes` (create, update, get, list, watch, delete) in the `gateway.networking.k8s.io` API group.
- InfrastructureManager changes:
  - Implement HTTPRoute reconciliation:
    - When Gateway API CRDs are installed and `spec.gateway.enabled` is true with a valid configuration, create or update an `HTTPRoute` resource (`<cluster>-httproute`) in the cluster namespace.
    - Set `parentRefs` to the configured Gateway reference.
    - Configure hostname and path routing to the OpenBao public Service on port `8200`.
    - If Gateway API CRDs are not installed, log and skip HTTPRoute reconciliation without failing the overall reconcile.
    - If Gateway is disabled or the configuration is incomplete (for example, missing hostname or Gateway name), delete any existing HTTPRoute and skip creation.
  - Implement Gateway CA ConfigMap management:
    - Maintain a `ConfigMap` named `<cluster>-tls-ca` containing the CA certificate in `ca.crt`, kept in sync with the CA Secret of the same name.
    - Ensure the `ConfigMap` has an `OwnerReference` to the `OpenBaoCluster` and is deleted when Gateway is disabled.
  - Add Gateway hostname to TLS SANs as part of the TLS lifecycle so that certificates are valid for the externally exposed hostname.
- Finalizer handling:
  - Delete `HTTPRoute` when `OpenBaoCluster` is deleted (or when Gateway support is disabled).
  - Handle invalid or incomplete Gateway configuration by cleaning up HTTPRoute resources without setting `Degraded` conditions.

Tests:

- Unit tests for HTTPRoute generation.
- envtest coverage for Gateway reconciliation and TLS SAN updates.
- E2E tests verifying traffic routing through Gateway API (requires Gateway controller in test cluster).

-----

## 5. UpgradeManager: Raft-Aware Rolling Updates

**Status: Implemented**

**Goals**

- Implement the upgrade state machine using StatefulSet partitions, leader detection, and safe step-down.
- Provide version validation, upgrade state persistence, and configurable timeouts.
- Ensure upgrades are resumable after Operator restarts.

**Tasks**

### 5.1 OpenBao API Client

- Implement `internal/openbao/client.go`:
  - `NewClient(baseURL, token, caCert)` constructor.
  - `Health(ctx) (*HealthResponse, error)` - calls `GET /v1/sys/health`.
  - `StepDown(ctx) error` - calls `PUT /v1/sys/step-down`.
  - `IsLeader(ctx) (bool, error)` - determines if connected node is leader.
  - TLS configuration trusting the cluster CA from `<cluster>-tls-ca` Secret.
  - Configurable timeouts (connection: 5s, request: 10s).

- Implement `internal/openbao/client_test.go`:
  - Table-driven unit tests with httptest mock server.
  - Test cases: healthy response, sealed response, leader/standby detection, network errors.

### 5.2 Version Validation

- Implement `internal/upgrade/version.go`:
  - `ValidateVersion(version string) error` - validates semver format.
  - `CompareVersions(from, to string) (VersionChange, error)` - returns upgrade/downgrade/same.
  - `IsDowngrade(from, to string) bool` - detects downgrades.
  - `VersionChange` enum: `VersionChangePatch`, `VersionChangeMinor`, `VersionChangeMajor`, `VersionChangeDowngrade`.

- Implement `internal/upgrade/version_test.go`:
  - Table-driven tests for version parsing and comparison.
  - Test cases: valid versions, invalid versions, patch/minor/major upgrades, downgrades.

### 5.3 Upgrade State Machine

- Update `internal/upgrade/manager.go`:
  - Add `OpenBaoClient` field for API calls.
  - Implement `Reconcile()` with full state machine:

  ```
  Phase 1: Detection
  - If Spec.Version == Status.CurrentVersion AND Status.Upgrade == nil, return (no upgrade needed).
  - If Status.Upgrade != nil, resume existing upgrade.
  - Otherwise, start new upgrade.
  
  Phase 2: Pre-upgrade Validation
  - Validate target version (semver format).
  - Check for downgrade (block if detected).
  - Verify all pods are Ready.
  - Verify quorum is healthy (call Health() on all pods, majority must be unsealed).
  - Verify single leader can be identified.
  - If spec.backup.preUpgradeSnapshot, trigger backup and wait.
  
  Phase 3: Initialize Upgrade
  - Create Status.Upgrade with TargetVersion, FromVersion, StartedAt.
  - Set Status.Phase = Upgrading.
  - Set Upgrading=True condition.
  - Patch StatefulSet partition = Replicas (lock updates).
  
  Phase 4: Pod-by-Pod Update
  For each pod from highest ordinal to 0:
    - Check if pod is leader via Health().
    - If leader: call StepDown(), wait for leadership transfer (with timeout).
    - Decrement partition to allow pod update.
    - Wait for pod Ready (with timeout).
    - Wait for Health() to show initialized=true, sealed=false (with timeout).
    - Update Status.Upgrade.CompletedPods and CurrentPartition.
  
  Phase 5: Finalization
  - Set Status.CurrentVersion = Spec.Version.
  - Clear Status.Upgrade (set to nil).
  - Set Status.Phase = Running.
  - Set Upgrading=False with Reason=UpgradeComplete.
  ```

### 5.4 Timeout Configuration

- Add timeout constants in `internal/upgrade/constants.go`:
  - `DefaultStepDownTimeout = 30 * time.Second`
  - `DefaultPodReadyTimeout = 5 * time.Minute`
  - `DefaultHealthCheckTimeout = 2 * time.Minute`
  - `DefaultRaftSyncTimeout = 2 * time.Minute`
  - `DefaultHealthCheckInterval = 5 * time.Second`

- Implement timeout enforcement via context deadlines.
- On timeout: halt upgrade, set Degraded=True with appropriate reason.

### 5.5 Resumability

- Implement resume logic in `Reconcile()`:
  - If `Status.Upgrade != nil` on entry:
    - Verify `Spec.Version == Status.Upgrade.TargetVersion`.
    - If changed mid-upgrade, clear Status.Upgrade and restart.
    - Otherwise, resume from `Status.Upgrade.CurrentPartition`.
    - Re-verify health of already-upgraded pods before continuing.

### 5.6 Status Updates

- Update `api/v1alpha1/openbaocluster_types.go`:
  - Add `UpgradeProgress` struct (if not already present from TDD).
  - Add `Upgrade *UpgradeProgress` field to Status.

- Implement status update helpers in `internal/upgrade/status.go`:
  - `SetUpgradeStarted(status, from, to)`
  - `SetUpgradeProgress(status, partition, completedPods)`
  - `SetUpgradeComplete(status)`
  - `SetUpgradeFailed(status, reason)`

### 5.7 Metrics

- Implement upgrade metrics in `internal/upgrade/metrics.go`:
  - `openbao_upgrade_status` gauge (0=none, 1=running, 2=success, 3=failed)
  - `openbao_upgrade_duration_seconds` histogram
  - `openbao_upgrade_pod_duration_seconds` histogram (labeled by pod)
  - `openbao_upgrade_stepdown_total` counter
  - `openbao_upgrade_stepdown_failures_total` counter
  - `openbao_upgrade_in_progress` gauge

### 5.8 Pause Integration

- Check `spec.paused` at start of Reconcile.
- If paused mid-upgrade: halt but preserve Status.Upgrade.
- On unpause: resume from saved state.

**Tests**

Unit tests (`internal/upgrade/*_test.go`):
- Version validation and comparison (table-driven).
- State machine transitions.
- Timeout handling.
- Resume logic.

envtest tests (`internal/controller/openbaocluster_controller_test.go`):
- Upgrade detection and Status.Upgrade initialization.
- StatefulSet partition management.
- Condition updates (Upgrading, Degraded).
- Pause during upgrade behavior.

E2E tests:
- Full upgrade from version A to B with leader step-down.
- Upgrade resumption after operator restart.
- Upgrade blocked on degraded cluster.
- Downgrade rejection.

**Acceptance Criteria**

- Upgrades proceed pod-by-pod in reverse ordinal order.
- Leader is always stepped down before its pod is updated.
- Upgrade state survives operator restart.
- Downgrades are blocked by default.
- Timeouts are enforced and failures are surfaced via Conditions.

-----

## 6. BackupManager: Snapshots to Object Storage

**Status: Implemented**

**Goals**

- Implement scheduled backups using OpenBao's Raft snapshot API and streaming to generic object storage.
- Support configurable retention policies (by count and age).
- Provide cloud-agnostic object storage interface with multiple authentication methods.

**Tasks**

### 6.1 Object Storage Client

- Implement `internal/storage/client.go`:
  - Interface definition:
    ```go
    type ObjectStorage interface {
        Upload(ctx context.Context, key string, body io.Reader, size int64) error
        Delete(ctx context.Context, key string) error
        List(ctx context.Context, prefix string) ([]ObjectInfo, error)
        Head(ctx context.Context, key string) (*ObjectInfo, error)
    }
    ```

- Implement `internal/storage/s3.go`:
  - S3-compatible implementation using AWS SDK v2.
  - Constructor: `NewS3Client(endpoint, bucket, region, credentials)`.
  - Support for static credentials, session tokens, and default credential chain.
  - Multipart upload for objects > 100MB (10MB parts).
  - TLS configuration with optional custom CA.

- Implement `internal/storage/credentials.go`:
  - `LoadCredentials(ctx, client, secretRef) (*Credentials, error)`
  - Parse Secret keys: `accessKeyId`, `secretAccessKey`, `sessionToken`, `region`, `caCert`.
  - Return nil credentials if secretRef is nil (use default chain).

### 6.2 Backup Naming

- Implement `internal/backup/naming.go`:
  - `GenerateBackupKey(pathPrefix, namespace, cluster string) string`
  - Format: `<pathPrefix>/<namespace>/<cluster>/<timestamp>-<uuid>.snap`
  - Timestamp: RFC3339 in UTC with colons replaced by dashes.
  - UUID: 8 hex characters from `crypto/rand`.

### 6.3 Backup Scheduling

- Implement `internal/backup/scheduler.go`:
  - Use `github.com/robfig/cron/v3` for cron parsing.
  - `NextSchedule(cronExpr string, from time.Time) (time.Time, error)`
  - `IsDue(cronExpr string, lastBackup, now time.Time) bool`
  - Missed schedule detection: trigger if `now - lastBackup > 2 * interval`.

- Add scheduling logic to `Reconcile()`:
  - Calculate next scheduled time and store in `Status.Backup.NextScheduledBackup`.
  - On Operator restart: recalculate from cron expression.

### 6.4 Backup Execution

- Update `internal/backup/manager.go`:
  - Implement `Reconcile()` with phases:
    1. Pre-flight checks (cluster healthy, no upgrade in progress, backup due, authentication configured).
    2. Ensure backup ServiceAccount exists (`<cluster-name>-backup-serviceaccount`).
    3. Create Kubernetes Job with backup executor container.
    4. Monitor Job status and update cluster status accordingly.
    5. Apply retention policy after successful backup.
    6. Finalize (clear BackingUp condition, update metrics).
- Implement `cmd/bao-backup/main.go` (backup executor):
  - Load configuration from environment variables and mounted files.
  - Authenticate to OpenBao (Kubernetes Auth or static token).
  - Discover leader via Health() API.
  - Stream snapshot: `GET /v1/sys/storage/raft/snapshot` -> object storage.
  - Verify upload via Head() request.
  - Exit with appropriate exit codes.

### 6.5 Retention Policy

- Implement `internal/backup/retention.go`:
  - `ApplyRetention(ctx, storage, prefix, policy) (deleted int, error)`
  - List objects, sort by timestamp.
  - Apply MaxCount limit (keep newest N).
  - Apply MaxAge limit (delete older than threshold).

### 6.6 Authentication

- Extend OpenBao client for snapshot API:
  - `Snapshot(ctx) (io.ReadCloser, error)` - calls `GET /v1/sys/storage/raft/snapshot`.
  - `KubernetesAuthLogin(ctx, role, jwtToken) (string, error)` - authenticates using Kubernetes Auth.
- Authentication methods (in order of preference):
  1. **Kubernetes Auth (Preferred):** Uses ServiceAccount token from `<cluster-name>-backup-serviceaccount`.
     - Configured via `spec.backup.kubernetesAuthRole`.
     - Tokens are automatically rotated by Kubernetes.
  2. **Static Token (Fallback):**
     - All clusters: Use token from `spec.backup.tokenSecretRef` Secret (must be explicitly configured).
     - Root tokens are not used for backup operations. Users must create a dedicated backup token with minimal permissions.
- ServiceAccount Management:
  - Operator automatically creates `<cluster-name>-backup-serviceaccount` when backups are enabled.
  - ServiceAccount is used by backup Jobs for Kubernetes Auth.

### 6.7 Status Updates

- Add `BackupStatus` struct to Status with:
  - `LastBackupTime`, `LastBackupSize`, `LastBackupDuration`, `LastBackupName`
  - `NextScheduledBackup`, `ConsecutiveFailures`, `LastFailureReason`

### 6.8 Concurrent Backup Prevention

- Check `BackingUp` condition before starting.
- Skip scheduled backups if upgrade in progress.

### 6.9 Metrics

- Implement backup metrics:
  - `openbao_backup_last_success_timestamp`, `openbao_backup_last_duration_seconds`
  - `openbao_backup_success_total`, `openbao_backup_failure_total`
  - `openbao_backup_consecutive_failures`, `openbao_backup_in_progress`

### 6.10 Webhook Validation

- Reject schedules more frequent than 15 minutes.
- Warn on schedules more frequent than 1 hour.
- Validate cron expression syntax and retention values.

**Tests**

Unit tests:
- Object storage client operations (table-driven).
- Backup key generation and scheduling logic.
- Retention policy enforcement.

envtest tests:
- BackupSchedule parsing and Status.Backup initialization.
- Condition updates (BackingUp, Degraded).

E2E tests (with MinIO):
- Full backup cycle: schedule -> stream -> upload -> verify.
- Retention enforcement.
- Backup during upgrade (should be skipped).

**Acceptance Criteria**

- Scheduled backups are executed via Kubernetes Jobs with the backup executor container.
- Backup executor streams directly to object storage without disk buffering.
- Backup naming follows predictable, sortable convention.
- Retention policies are enforced after successful uploads.
- Concurrent backups are prevented.
- Kubernetes Auth is supported and preferred over static tokens.
- ServiceAccount is automatically created for backup Jobs.
- Self-init clusters without authentication configuration show clear error condition.

-----

## 7. Observability: Metrics and Logging

**Status: Partially Implemented (Logging only; Metrics not implemented)**

**Goals**

- Expose the metrics and structured logs defined in the TDD and FRs.

**Tasks**

- Metrics:
  - Implement OpenTelemetry metrics:
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

**Status: Implemented**

**Goals**

- Ensure safe multi-tenant behavior and RBAC as described in the docs.

**Tasks**

- Resource scoping:
  - Verify all created resources (Secrets, ConfigMaps, StatefulSets, Services) are namespaced and uniquely named per `OpenBaoCluster`.
  - Avoid any global/shared resources across clusters.
  - All resources use naming convention `<cluster-name>-<suffix>` (e.g., `prod-cluster-config`, `prod-cluster-unseal-key`).
  - Resources are labeled with `openbao.org/cluster=<cluster-name>` for identification.
- RBAC:
  - Define `ClusterRole`/`Role` and `ClusterRoleBinding`/`RoleBinding` manifests to:
    - Allow the operator access to Secrets, StatefulSets, Pods, `pods/exec`, ConfigMaps in relevant namespaces.
    - Allow namespace-scoped teams to manage only their `OpenBaoCluster` CRs.
  - Three ClusterRoles provided for user RBAC:
    - `openbaocluster-admin-role`: Full access to OpenBaoCluster resources.
    - `openbaocluster-editor-role`: Create, update, delete access.
    - `openbaocluster-viewer-role`: Read-only access.
  - Example namespace-scoped RoleBinding manifests provided in `config/rbac/namespace_scoped_example.yaml`.
- Rate limiting:
  - Configure controller-runtime rate limiting and per-controller `MaxConcurrentReconciles` to prevent hot loops from overwhelming the API server in multi-tenant environments, while still supporting at least 10 concurrent `OpenBaoCluster` resources under normal load.
  - Controller configured with `MaxConcurrentReconciles: 3` and exponential backoff rate limiter (1s base, 60s max).
- Resource ownership and watches:
  - All created resources (StatefulSets, Services, ConfigMaps, Secrets, ServiceAccounts, Ingresses) have OwnerReferences pointing to the parent `OpenBaoCluster` for automatic garbage collection.
  - Controller watches all owned resource types, triggering reconciliation when child resources are modified out-of-band.
  - OwnerReferences use `controller=true` to mark the OpenBaoCluster as the controlling owner.

Tests:

- envtest tests verify:
  - Multiple clusters in different namespaces reconcile independently (FR-MT-01).
  - Multiple clusters per namespace with no cross-impact (FR-MT-02).
  - Failure in one cluster does not prevent reconciliation of others (FR-MT-03).
  - Resources are uniquely named per cluster preventing cross-tenant sharing (FR-MT-05).
- Unit tests for:
  - Resource naming uniqueness across clusters in same namespace.
  - Namespace isolation for same-named clusters in different namespaces.
  - Resource labeling with cluster identifiers.
  - OwnerReferences are set on all created resources (StatefulSet, ConfigMap, Services, Secrets, ServiceAccount).

**Acceptance Criteria:**

- All resources created by the operator are namespaced to the `OpenBaoCluster` namespace.
- Resource names include the cluster name as a prefix to prevent collisions.
- RBAC roles support namespace-scoped user access patterns.
- Rate limiting prevents reconciliation hot-loops.
- All resources have OwnerReferences for automatic garbage collection when the parent OpenBaoCluster is deleted.
- Controller watches owned resources to respond to out-of-band modifications.

**Documentation:**

- `docs/usage-guide.md` Section 10 provides comprehensive multi-tenancy security guidance including:
  - RBAC configuration for tenant isolation
  - Secret access isolation using policy engines
  - NetworkPolicy templates for cluster isolation
  - ResourceQuota examples
  - Backup credential isolation patterns
  - Pod Security Standards configuration
  - Audit logging recommendations
  - Multi-tenancy deployment checklist
- `docs/thread-model.md` Section 6 covers multi-tenancy threats and mitigations
- Example namespace-scoped RoleBindings in `config/samples/namespace_scoped_openbaocluster-rbac.yaml`

-----

## 9. Webhooks and Advanced Validation (Optional but Recommended)

**Status: Implemented (as part of Phase 4)**

**Note:** The validating webhook was implemented earlier than originally planned and is included in Phase 4.

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

**Status: Partially Implemented (CI workflows defined; E2E tests scaffolded)**

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

**Status: Partially Implemented (Usage guide exists; being updated to reflect implementation)**

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
