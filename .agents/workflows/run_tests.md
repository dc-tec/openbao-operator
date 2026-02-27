---
description: Run unit and E2E tests for the OpenBao Operator
---
This workflow guides local test execution for standard changes and refactors.

# Fast Local Tests

Run standard local tests:

```bash
make test
```

# CI-Equivalent Unit + Integration

Run the CI-style non-E2E test lane:

```bash
make test-ci
```

# Integration Tests (envtest)

```bash
make test-integration
```

# E2E Tests

E2E tests require a Kubernetes cluster (typically Kind).

To run all E2E tests:

```bash
make test-e2e
```

To run a specific subset of E2E tests (e.g., only Backup related):

```bash
make test-e2e E2E_FOCUS="Backup"
```

To run a specfic subset of E2E tests using test labels (eg., only smoke, slow, backup)

```bash
make test-e2e E2E_LABEL_FILTER="smoke"
```

To run E2E tests in parallel (e.g., 4 nodes):

```bash
make test-e2e E2E_PARALLEL_NODES=4
```

# Architecture Dependency Sanity (for refactors)

After structural/package-boundary changes:

```bash
make report-internal-deps
```
