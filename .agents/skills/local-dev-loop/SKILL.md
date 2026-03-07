# Skill: local-dev-loop

Use this skill when you need to choose the right local iteration loop for OpenBao Operator work, especially when Air, Delve, Tilt, gotestsum, or benchstat are involved.

## Pick the right loop

- Use `make air-controller` or `make air-provisioner` for the fastest edit-build-restart loop against the current kubeconfig.
- Use `make debug-controller`, `make debug-provisioner`, or `make debug-test PKG=... TEST=...` when you need breakpoints, stepping, or goroutine inspection with Delve.
- Use `make tilt-up` when the repro depends on in-cluster behavior such as webhooks, RBAC, image wiring, or multi-resource logs.
- Use `make test-sum` or `make test-integration-sum` when you want readable test output plus JUnit and coverage artifacts under `dist/test/`.
- Use `make bench-save` followed by `make bench-compare OLD=... NEW=...` when validating performance-sensitive changes.

## Operating rules

1. Prefer the repo's `make` targets over calling `air`, `dlv`, `gotestsum`, or `benchstat` directly.
2. Start with the narrowest loop that can reproduce the issue, then escalate to Tilt only if local binaries are insufficient.
3. Keep benchmark comparisons scoped and repeatable: same package, same benchmark filter, same count.
4. When a debugging or benchmark run produces useful artifacts, cite the generated files in `dist/test/` or `dist/bench/`.
