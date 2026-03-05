---
description: Lint code and verify linters configuration
---
This workflow helps maintain code quality by running linters and verifying configurations.

# CI-Equivalent Lint

Run the same lint flow used in CI:

```bash
make lint-ci
```

# Optional Auto-Fix

If you want local auto-fixes for lint suggestions first:

```bash
make lint-fix
```

# Architecture Dependency Report (for structural changes)

When changing package boundaries/imports, generate the internal dependency report:

```bash
make report-internal-deps
```

Then inspect:

- `dist/architecture/internal-dependency-report.md`
- `hack/architecture/dependency-policy-exceptions.tsv`
