---
trigger: always_on
glob: "{api,config,charts,dist,hack}/**/*"
description: Generated artifacts workflows (CRDs/Helm/RBAC/golden files)
---

# Generated Artifacts

This repo has multiple generated outputs that must stay in sync with their sources.
See `docs/contributing/standards/generated-artifacts.md` for the canonical guide.

## Rules of Thumb

- Prefer updating the **source**, then regenerating outputs.
- Generators must be **deterministic** (stable ordering) to keep diffs reviewable.
- If a PR changes a generated file unexpectedly, re-run the corresponding `make` target and verify intent.

## Common Sync Targets

```sh
make verify-generated   # CI-equivalent check, no file modifications expected
make manifests generate # CRDs + deepcopy (controller-gen)
make helm-sync          # charts/openbao-operator (CRDs + install.yaml.tpl)
make rbac-sync          # config/rbac/provisioner_delegate_clusterrole.yaml
make test-update-golden # internal/config/testdata golden HCL
```

## Heuristics (When to Run What)

- Touching `api/` or `config/crd/`: run `make manifests generate` (and usually `make helm-sync`).
- Touching `dist/install.yaml` or `charts/openbao-operator/`: run `make helm-sync`.
- Touching `internal/provisioner/rbac.go` or `hack/gen-rbac/`: run `make rbac-sync`.
- Touching `internal/config/*`: consider `make test-update-golden`.
