---
description: Validate internal architecture boundaries against the layered model
---

Use this workflow after any change that moves code across packages, alters constructors, or changes imports.

# 1) Generate dependency report

```bash
make report-internal-deps
```

# 2) Review report output

Open:

- `dist/architecture/internal-dependency-report.md`
- `dist/architecture/internal-dependency-edges.tsv`

Confirm:

- Cycle check is acyclic
- Policy warnings are `None` (or intentionally justified)
- Hotspots stay within current targets (`internal/constants` fan-in, `internal/controller/openbaocluster` fan-out)

# 3) Verify boundary intent from layer model

Check for prohibited edges:

- Services/adapters importing `internal/controller/*`
- Adapters importing service/manager packages
- Reintroduction of `internal/interfaces`

Layer intent:

- Controllers (`internal/controller/*`) delegate orchestration to `internal/app/*`
- Services consume ports (`internal/port/*`) and shared utilities
- Adapters implement ports and avoid service/controller imports

# 4) Validate code health

```bash
make lint-config lint
go test ./...
```
