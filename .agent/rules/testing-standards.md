---
trigger: always_on
glob: "**/*_test.go"
description: Testing standards for the OpenBao Operator
---

# Testing Standards

See [Testing Guide](docs/contributing/testing.md) for full documentation.

## Test Requirements by Change Type

| Change Type | Required Tests |
|-------------|----------------|
| Logic in `internal/` | Table-driven unit tests |
| HCL generation | Updated golden files (`make test-update-golden`) |
| Reconciliation flows | At least one envtest integration test |
| Upgrade/backup changes | At least one E2E scenario |

## Patterns

1. **Table-Driven Tests**: Use for unit tests in `internal/`
2. **Golden Files**: For HCL output verification (`internal/config/testdata/`)
3. **EnvTest**: For integration tests behind `-tags=integration` (prefer `test/integration/`)
4. **Ginkgo/Gomega**: For E2E tests with Kind

## Commands

```sh
make test               # Unit tests (fast, no envtest)
make test-integration   # Envtest-based integration tests (-tags=integration)
make test-ci            # Unit + integration tests (CI-equivalent)
make test-update-golden # Update HCL golden files
make test-e2e           # E2E tests (requires Kind)
```

## Rules

1. New `internal/` logic MUST have unit tests
2. Golden file changes MUST be reviewed carefully
3. Test helpers are OK to duplicate slightly for readability
4. Use `t.Helper()` in test helper functions
5. Use `require` for fatal assertions, `assert` for non-fatal
