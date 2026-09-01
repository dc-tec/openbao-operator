---
trigger: always_on
glob: "**/*_test.go"
description: Testing standards for the OpenBao Operator
---

# Testing Standards

See [Testing Guide](../../website/content/contribute/testing.md) for full documentation.

## Test Requirements by Change Type

| Change Type | Required Tests |
|-------------|----------------|
| Logic in `internal/` | Table-driven unit tests |
| Kubernetes object builders / fake-client flows | Untagged fast contract tests |
| HCL generation | Updated golden files (`make test-update-golden`) |
| Reconciliation flows | At least one envtest integration test |
| Controller setup / watch behavior | Envtest integration that starts a manager |
| Upgrade/backup changes | At least one E2E scenario |
| Manifest compatibility / policy bundles | Untagged static contract tests under `test/manifests/` or `test/utils/` |

## Patterns

1. **Table-Driven Tests**: Use for unit tests in `internal/`
2. **Golden Files**: For HCL output verification (`internal/adapter/config/testdata/`)
3. **Fast Kubernetes Contract Tests**: Keep fake-client and manifest contract tests untagged
4. **EnvTest**: Use behind `-tags=integration` for real API-server semantics (prefer `test/integration/`)
5. **Ginkgo/Gomega**: Use for E2E tests with Kind

## Commands

```sh
make test               # Unit tests (fast, no envtest)
make test-sum           # Unit tests with gotestsum output + JUnit/coverage artifacts in dist/test/
make test-integration   # Envtest-based integration tests (-tags=integration)
make test-integration-sum # Envtest tests with gotestsum output + JUnit/coverage artifacts in dist/test/
make test-ci            # Unit, integration, and cluster-independent E2E support tests (CI-equivalent)
make ci-core            # Main pre-PR local gate (everything except E2E)
make test-update-golden # Update HCL golden files
make test-e2e           # E2E tests (requires Kind)
make bench-save         # Save targeted benchmark output under dist/bench/
make bench-compare OLD=... NEW=... # Compare benchmark runs with benchstat
```

## Rules

1. New `internal/` logic MUST have unit tests
2. Golden file changes MUST be reviewed carefully
3. Test helpers are OK to duplicate slightly for readability
4. Use `t.Helper()` in test helper functions
5. Use `require` for fatal assertions, `assert` for non-fatal
6. Prefer `test-sum` / `test-integration-sum` when you want readable local output or CI-style artifacts
7. Reserve `//go:build integration` and `_integration_test.go` for envtest-backed tests only
8. Use envtest when the behavior depends on validation, defaulting, status subresources,
   `Generation` / `ResourceVersion`, admission, SSA, owner references, or real API-server semantics
9. Controller tests that claim `SetupWithManager()`, watch behavior, cache/index behavior, or
   event-driven reconciliation MUST start a manager and register the reconciler
10. Direct `Reconcile(...)` tests are fine for fast orchestration coverage, but they are not a
    substitute for manager-driven controller integration tests
11. Prefer `test/integration/` for shared envtest suites; keep package-local envtest only when
    colocated fixtures materially improve clarity
12. Keep static manifest / compatibility tests untagged under `test/manifests/` or `test/utils/`
13. Performance-sensitive changes SHOULD include targeted benchmarks and `benchstat` comparisons when evaluating regressions
