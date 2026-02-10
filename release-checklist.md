# 0.1.0 Release Readiness Epic

Owner: @roelc (sole maintainer)

## P0 (must complete before tag)

- [x] **API/CRD contract freeze**
  - Scope: schema defaults, validation rules, status condition/reason semantics, backward compatibility.
  - Exit criteria:
    - [x] CRDs reviewed for defaulting/validation consistency.
    - [x] Backward-compat manifests reconcile cleanly.
    - [x] Status condition contract documented and stable.

- [x] **Controller determinism + idempotency audit**
  - Scope: `workload`, `adminops`, `status`, operation lock behavior, retries/requeues.
  - Exit criteria:
    - [x] Reconcile paths are idempotent under duplicate events.
    - [x] No race-prone state transitions across split reconcilers.
    - [x] Lock acquire/release semantics hold across controller restart.

- [x] **Failure-injection coverage (upgrade/backup/restore)**
  - Scope: transient storage/auth/network failures, stepdown timing, stuck jobs, restart in-flight.
  - Exit criteria:
    - [x] Induced failure tests assert retry/rollback/fail-fast behavior.
    - [x] No stale status fields after recovery.
    - [x] Deterministic completion/failure outcomes.

- [x] **Security hardening audit**
  - Scope: RBAC least privilege, token scope/audience, secret handling, log redaction.
  - Exit criteria:
    - [x] RBAC permissions justified and minimal.
    - [x] No secret/token leakage in logs/events/status.
    - [x] Auth paths validated for JWT and token fallback behavior.

- [x] **Release reproducibility**
  - Scope: build determinism, pinned versions, provenance artifacts.
  - Exit criteria:
    - [x] Toolchain and image versions pinned and consistent.
    - [x] Repeated builds produce expected artifacts.
    - [x] SBOM/signing/provenance pipeline green.
    - [x] Hardened profile e2e tests with full verification

## P1 (strongly recommended before tag)

- [x] **Compatibility matrix verification**
  - Scope: supported operator/OpenBao combinations.
  - Exit criteria:
    - [x] Matrix runs pass for supported versions.
    - [x] Docs matrix matches tested reality.

- [ ] **Observability + runbook readiness**
  - Scope: metrics, alerts, actionable runbooks for top failure modes.
  - Exit criteria:
    - [ ] Alerts mapped to concrete remediation steps.
    - [ ] Backup/restore/upgrade scenarios covered.
    - [ ] `make verify-grafana-dashboards` passes (dashboard JSON + metric reference validation).

- [ ] **Performance baseline**
  - Scope: reconcile latency, job durations, control-plane churn.
  - Exit criteria:
    - [ ] Baseline captured at `hack/perf/baseline/kind-v1.34.3-baseline.json`.
    - [ ] Regression thresholds defined at `hack/perf/thresholds/kind-v1.34.3.yaml`.
    - [ ] `make verify-perf` passes locally.
    - [ ] CI `perf-gate` runs on targeted PR paths and on nightly/release workflows.

## P2 (pre-0.1.0 hardening unless time allows)

- [ ] **Targeted refactors for maintainability**
  - Scope: simplify complex state flows, remove duplicate logic.
  - Exit criteria:
    - [ ] Behavior-preserving refactors merged with tests.
    - [ ] Reduced branching/duplication in critical paths.

- [ ] **Final docs alignment pass**
  - Scope: user + architecture docs for edge cases and failure behavior.
  - Exit criteria:
    - [ ] Docs match implemented behavior and tested scenarios.
    - [ ] Operational caveats explicit.
    - [ ] Provided samples / recipes are valid 

## Release gate checklist

- [ ] All P0 items complete.
- [ ] P1 complete or explicitly deferred with written risk acceptance.
- [ ] CI green: lint, unit, integration, targeted e2e.
- [ ] 24h burn-in run (periodic backup/restore + upgrade smoke) completed.
- [ ] Tag `v0.1.0` + release notes finalized.
