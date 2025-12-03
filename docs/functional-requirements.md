# Functional Requirements Specification

## 1. Security (TLS Authority)
* **FR-SEC-01:** The Operator MUST generate a self-signed Root CA if one does not exist.
* **FR-SEC-02:** The Operator MUST issue leaf certificates for the OpenBao cluster with correct SANs (`*.openbao.svc`, `localhost`, IP).
* **FR-SEC-03:** The Operator MUST automatically rotate certificates 7 days (configurable) before expiry.
* **FR-SEC-04:** The Operator MUST trigger a hot-reload (SIGHUP) on OpenBao pods when certs are updated.

## 2. Infrastructure (Bootstrapping)
* **FR-INF-01:** The Operator MUST create a StatefulSet, Service, and ConfigMap based on the CR.
* **FR-INF-02:** The Operator MUST inject `retry_join` stanzas into the ConfigMap that point to the deterministic DNS names of the replicas.
* **FR-INF-03:** The cluster MUST bootstrap into a Quorum (Health status 200) without manual intervention, using static auto-unseal managed by the Operator.

## 3. Operations (Upgrades)
* **FR-OPS-01:** The Operator MUST detect a version change in the CRD.
* **FR-OPS-02:** The Operator MUST NEVER update a Leader pod. It must force a step-down first.
* **FR-OPS-03:** The Operator MUST pause the upgrade if the cluster health is degraded.
* **FR-OPS-04:** The Operator MUST provide a maintenance mode via `OpenBaoCluster.Spec.Paused` that stops reconciliation for that cluster while preserving the ability to perform cleanup on delete.

## 4. Networking & External Access
* **FR-NET-01:** The Operator MUST support configuring external access to OpenBao via Kubernetes `Service` (e.g., LoadBalancer) and/or `Ingress` resources based on the `OpenBaoCluster` spec.
* **FR-NET-02:** The Operator MUST ensure that any externally exposed hostnames are reflected in the TLS SANs (via `ExtraSANs` or equivalent configuration).
* **FR-NET-03:** The Operator MUST document recommended patterns for integrating with Ingress controllers and/or service meshes without breaking mTLS.

## 5. Disaster Recovery (Backup)
* **FR-DR-01:** The Operator MUST accept a Cron schedule (e.g., "0 3 * * *").
* **FR-DR-02:** The Operator MUST stream the Raft snapshot directly to object storage without buffering to disk.

## 6. Multi-Tenancy & Isolation
* **FR-MT-01:** The Operator MUST support managing multiple `OpenBaoCluster` resources in a single Kubernetes cluster.
* **FR-MT-02:** The Operator MUST support multiple `OpenBaoCluster` resources per namespace, with no cross-impact between clusters.
* **FR-MT-03:** A failure or misconfiguration in one `OpenBaoCluster` MUST NOT prevent reconciliation of other `OpenBaoCluster` resources.
* **FR-MT-04:** The Operator MUST scope RBAC so that namespace-scoped users can manage only the `OpenBaoCluster` resources in their namespaces (when desired by cluster admins).
* **FR-MT-05:** The Operator MUST avoid sharing Secrets or ConfigMaps between different `OpenBaoCluster` instances by name; each cluster MUST have uniquely named resources to prevent cross-tenant impact.

## 7. Non-Functional Requirements
* **FR-NFR-01:** Under normal load, the Operator SHOULD reconcile changes to an `OpenBaoCluster` within 30 seconds.
* **FR-NFR-02:** The Operator MUST implement idempotent reconciliation so that re-running the Reconcile loop does not cause unintended side effects.
* **FR-NFR-03:** The Operator MUST implement exponential backoff for transient errors (e.g., OpenBao API unavailable, object storage temporarily unreachable).
* **FR-NFR-04:** The Operator MUST support at least 10 concurrent `OpenBaoCluster` resources per Kubernetes cluster without degradation beyond agreed SLOs.
* **FR-NFR-05:** The Operator Pod resource usage (CPU/memory) SHOULD be bounded and documented for typical cluster sizes (e.g., up to 5 OpenBao nodes per cluster).
* **FR-NFR-06:** The Operator MUST implement rate limiting and backoff at the controller level to prevent hot reconcile loops from overloading the API server in multi-tenant scenarios.

## 8. Observability & Diagnostics
* **FR-OBS-01:** The Operator MUST expose basic metrics (e.g., reconciliation duration, reconciliation failures, number of managed `OpenBaoCluster` resources) via a standard HTTP endpoint suitable for scraping by common monitoring systems.
* **FR-OBS-02:** The Operator MUST emit structured logs including `namespace`, `name` of the `OpenBaoCluster`, and a correlation identifier per reconciliation loop.
* **FR-OBS-03:** The Operator MUST surface critical failures (e.g., upgrade failure, backup failure) via `Status.Conditions` on the `OpenBaoCluster` resource.
* **FR-OBS-04:** The Operator MUST NOT interfere with or override OpenBao's own telemetry configuration; instead it SHOULD allow users to configure OpenBao telemetry via the CRD and rely on OpenBao to expose telemetry to their monitoring system of choice.
* **FR-OBS-05:** The Operator MUST expose metrics sufficient to drive alerts on cluster readiness, upgrade status, backup success/failure, and reconciliation errors.

## 9. Compatibility & Support Matrix
* **FR-COMP-01:** The Operator MUST be cloud-agnostic and not rely on provider-specific APIs for core functionality.
* **FR-COMP-02:** The Operator MUST support generic object storage for backups (e.g., S3-compatible, GCS-compatible, Azure Blob-compatible) via configurable endpoints and credentials.
* **FR-COMP-03:** The Operator MUST document the minimum supported Kubernetes and OpenBao versions and MUST validate unsupported versions with clear error messages in `Status.Conditions`.
