# E2E Case Catalog

Generated from `ginkgo outline` for the files under `test/e2e/`.

Notes:
- Suite and spec inventory comes from `ginkgo outline`; `case:` and `covers:` labels are the stable tracking fields.
- `steps` are optional recorded checkpoints derived from literal `By(...)` text that `ginkgo outline` can see.
- Missing checkpoints do not imply missing coverage.
- `case:` labels become the primary catalog IDs when present; otherwise a generated fallback ID is used.
- Use `cases.json` for automation; use the suite pages for human review.

## Summary

- Files: `18`
- Specs: `86`
- Explicit case IDs: `16`
- Coverage tags: `42`

## Suites

| Suite | Cases | Tracked | Pending | Labels | Source |
| --- | ---: | ---: | ---: | --- | --- |
| [Cluster Lifecycle: Deletion Policy](suites/Cluster_DeletionPolicy_test.md) | 3 | 3 | 0 | `lifecycle`, `cluster`, `deletion` | `test/e2e/Cluster_DeletionPolicy_test.go` |
| [Cluster Lifecycle](suites/Cluster_Lifecycle_test.md) | 7 | 0 | 0 | `lifecycle`, `cluster`, `profile-development`, `scaling`, `autopilot`, `smoke`, `critical`, `tenant` | `test/e2e/Cluster_Lifecycle_test.go` |
| [Hardened profile (External TLS + Transit auto-unseal + SelfInit)](suites/Cluster_Profile_Hardened_test.md) | 5 | 0 | 0 | `profile-hardened`, `security`, `cluster`, `upgrade`, `bluegreen`, `hardened`, `rolling` | `test/e2e/Cluster_Profile_Hardened_test.go` |
| [Cluster Runtime Controls](suites/Cluster_Runtime_Controls_test.md) | 4 | 4 | 0 | `lifecycle`, `cluster`, `runtime` | `test/e2e/Cluster_Runtime_Controls_test.go` |
| [ACME TLS (OpenBao native ACME client)](suites/Cluster_TLS_ACME_test.md) | 2 | 0 | 0 | `tls`, `security`, `slow` | `test/e2e/Cluster_TLS_ACME_test.go` |
| [Cluster TLS Lifecycle](suites/Cluster_TLS_Lifecycle_test.md) | 1 | 1 | 0 | `tls`, `cluster`, `lifecycle` | `test/e2e/Cluster_TLS_Lifecycle_test.go` |
| [GitOps contract (Argo-like apply)](suites/GitOps_Contract_test.md) | 1 | 0 | 0 | `gitops`, `contract` | `test/e2e/GitOps_Contract_test.go` |
| [Manager Resilience](suites/Manager_Resilience_test.md) | 3 | 3 | 0 | `manager`, `cluster` | `test/e2e/Manager_Resilience_test.go` |
| [Manager](suites/Operator_Manager_test.md) | 2 | 0 | 0 | `manager`, `critical`, `smoke` | `test/e2e/Operator_Manager_test.go` |
| [OpenShift Platform](suites/Platform_OpenShift_test.md) | 2 | 0 | 0 | `openshift`, `platform` | `test/e2e/Platform_OpenShift_test.go` |
| [Security Guardrails](suites/Security_Guardrails_test.md) | 20 | 0 | 0 | `security`, `critical`, `admission`, `config`, `pentest`, `tokens`, `rbac`, `tamper` | `test/e2e/Security_Guardrails_test.go` |
| [Tenant Data Isolation](suites/Tenant_Data_Isolation_test.md) | 1 | 1 | 0 | `security`, `tenant`, `tenancy` | `test/e2e/Tenant_Data_Isolation_test.go` |
| [Tenant Isolation](suites/Tenant_Isolation_test.md) | 6 | 0 | 0 | `security`, `tenant`, `tenancy`, `critical`, `single-tenant` | `test/e2e/Tenant_Isolation_test.go` |
| [Upgrade Strategies: Operation Lock Contention](suites/Upgrade_Operation_Lock_test.md) | 1 | 1 | 0 | `upgrade`, `backup`, `operation-lock`, `slow` | `test/e2e/Upgrade_Operation_Lock_test.go` |
| [Upgrade Strategies](suites/Upgrade_Strategies_test.md) | 14 | 0 | 0 | `upgrade`, `upgrades`, `cluster`, `slow`, `bluegreen`, `verification`, `failure`, `gateway`, `requires-gateway-api`, `tls-passthrough`, `rollback`, `rolling`, `recovery`, `snapshot`, `chaos`, `guardrails`, `validation` | `test/e2e/Upgrade_Strategies_test.go` |
| [Upgrade Strategies: Blue/Green Drift](suites/Upgrade_Target_Drift_test.md) | 1 | 1 | 0 | `upgrade`, `bluegreen`, `slow` | `test/e2e/Upgrade_Target_Drift_test.go` |
| [Security: Anti-Tamper Policy](suites/anti_tamper_policy_test.md) | 2 | 2 | 0 | `security`, `tamper`, `cluster`, `slow` | `test/e2e/anti_tamper_policy_test.go` |
| [DR: Storage Providers Backup & Restore](suites/backup_restore_test.md) | 11 | 0 | 0 | `dr`, `backup`, `restore`, `storage-providers`, `nightly`, `slow`, `failure-injection` | `test/e2e/backup_restore_test.go` |

## Coverage Tags

| Coverage | Cases |
| --- | ---: |
| `deletion-policy` | 3 |
| `anti-tamper` | 2 |
| `pvc-cleanup` | 2 |
| `recoverability-secret-cleanup` | 2 |
| `backup-queueing` | 1 |
| `bluegreen-drift` | 1 |
| `cert-replacement` | 1 |
| `configmap-protection` | 1 |
| `controller-failover` | 1 |
| `controller-outage` | 1 |
| `controller-restart` | 1 |
| `data-plane-isolation` | 1 |
| `existing-cluster-adoption` | 1 |
| `external-service` | 1 |
| `idempotent-reconcile` | 1 |
| `ingress` | 1 |
| `leader-election` | 1 |
| `network-isolation` | 1 |
| `observability-metrics` | 1 |
| `operation-lock` | 1 |
| `paused-reconcile` | 1 |
| `paused-status` | 1 |
| `pod-rollout` | 1 |
| `pod-stability` | 1 |
| `post-failover-reconcile` | 1 |
| `post-outage-reconcile` | 1 |
| `pvc-retention` | 1 |
| `recoverability-secret-retention` | 1 |
| `restart-at` | 1 |
| `rolling-upgrade` | 1 |
| `scale-reconcile` | 1 |
| `secret-regeneration` | 1 |
| `stale-green-cleanup` | 1 |
| `statefulset-protection` | 1 |
| `target-revision-drift` | 1 |
| `telemetry` | 1 |
| `tenant-isolation` | 1 |
| `tls-hot-reload` | 1 |
| `tls-lifecycle` | 1 |
| `tls-san` | 1 |
| `tls-secret-cleanup` | 1 |
| `tls-verification` | 1 |
