---
description: Run unit and E2E tests for the OpenBao Operator
---
This workflow guides you through running unit tests and End-to-End (E2E) tests for the OpenBao Operator.

# Unit Tests

To run the standard unit tests (excluding E2E), use the following command:

```bash
make test
```

# E2E Tests

E2E tests require a Kubernetes cluster (Kind). The Makefile handles the setup automatically.

To run all E2E tests:

```bash
make test-e2e
```

To run a specific subset of E2E tests (e.g., only Backup related):

```bash
make test-e2e E2E_FOCUS="Backup"
```

To run E2E tests in parallel (e.g., 4 nodes):

```bash
make test-e2e E2E_PARALLEL_NODES=4
```
