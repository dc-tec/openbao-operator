---
title: Test a change
description: Choose the lowest-cost test layer that proves the changed behavior, then expand with risk.
eyebrow: Contribute
weight: 3
verifiedBy:
  - Makefile
  - test/e2e/suites.yaml
  - hack/perf/v2/scenarios.yaml
---

Start at the lowest-cost layer that can prove the contract. Move upward when the behavior depends on Kubernetes API semantics, controller wiring, or a real workload.

| Change | First useful layer | What it proves |
| --- | --- | --- |
| Pure Go logic, parsers, renderers, helpers | Unit tests | Deterministic in-process behavior |
| Builders, manifests, patches, fake-client contracts | Unit and focused package tests | Emitted resource shape without API-server semantics |
| Reconciliation, finalizers, status, admission, defaulting | EnvTest integration | Real API-server behavior |
| Networking, storage, upgrades, backup, restore, workload startup | Kind E2E | Controller and workload behavior in a cluster |
| Platform compatibility, disaster recovery, performance | Scheduled or focused environment validation | Evidence for environment-specific assumptions |

The controller-runtime fake client is not a replacement for the API server. Use EnvTest when the test depends on validation, defaulting, subresources, watches, cache wiring, `Generation`, or `ResourceVersion`.

{{< command label="verify" title="Run the baseline before review" >}}
devenv test
devenv tasks run operator:bootstrap
devenv tasks run operator:doctor
devenv tasks run operator:ci-core
{{< /command >}}

## Run specialized lanes

- Use `make fuzz` for parsers, renderers, auth, or normalization code. Set `FUZZTIME` or `FUZZ_TARGET_FILTER` for a focused run.
- Use `make verify-perf` for changes that may affect reconcile cost, convergence, startup, or lifecycle timing.
- Use `make test-e2e-existing` for OpenShift and other platform-specific checks that Kind cannot reproduce.
- Use `make test-e2e-operator-upgrade` when controllers, CRDs, Helm, profiles, or migrations must preserve resources made by the prior stable operator.
- Use the manual HSM lane for KMIP and PKCS#11 paths that require test-only provider fixtures.

E2E suites are declared in `test/e2e/suites.yaml`. Update the suite owner, risk tier, isolation class, labels, coverage tags, CI lane, nightly policy, and parallelism whenever an E2E spec changes ownership or scope. Then run `make verify-e2e-manifest`.

Set `E2E_FAIL_ON_EMPTY=true` with label filters. Write Ginkgo JSON and JUnit reports so a failed lane retains selected specs, failure details, and slow-test evidence.

```sh
make test-e2e-ci \
  E2E_LABEL_FILTER='lifecycle && !openshift' \
  E2E_JUNIT_REPORT=artifacts/e2e-reports/local/junit.xml \
  E2E_JSON_REPORT=artifacts/e2e-reports/local/ginkgo.json \
  E2E_FAIL_ON_EMPTY=true
```

Continue with [CI routing]({{< relref "/contribute/ci.md" >}}).
