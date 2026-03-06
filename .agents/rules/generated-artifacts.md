---
trigger: always_on
glob: "{api,config,charts,dist,hack}/**/*"
description: Generated artifacts workflows (CRDs/Helm/RBAC/golden files)
---

# Generated Artifacts

This repo has multiple generated outputs that must stay in sync with their sources.
See `docs/contributing/standards/generated-artifacts.md` for the canonical guide.
This includes the committed Go `vendor/` tree.

## Rules of Thumb

- Prefer updating the **source**, then regenerating outputs.
- Generators must be **deterministic** (stable ordering) to keep diffs reviewable.
- If a PR changes a generated file unexpectedly, re-run the corresponding `make` target and verify intent.

## Common Sync Targets

```sh
make verify-generated   # CI-equivalent check, no file modifications expected
make verify-vendor      # vendor/ matches go.mod and go.sum
make manifests generate # CRDs + deepcopy (controller-gen)
make helm-sync          # charts/openbao-operator (CRDs + install.yaml.tpl)
make generate-ast-rules # generate architecture-boundary rules from policy
make verify-arch-policy # verify generated ast rules are up to date
make test-update-golden # internal/adapter/config/testdata golden HCL
```

## Heuristics (When to Run What)

- Touching `api/` or `config/crd/`: run `make manifests generate` (and usually `make helm-sync`).
- Touching `go.mod` or `go.sum`: run `make verify-vendor` (or `go mod vendor`) and commit `vendor/` updates.
- Touching `dist/install.yaml` or `charts/openbao-operator/`: run `make helm-sync`.
- Touching `.ast-grep/policy/architecture-boundaries.yml`: run `make generate-ast-rules verify-arch-policy`.
- Touching `internal/service/provisioner/rbac.go` or `hack/gen-rbac/`: run `make rbac-sync`.
- Touching `internal/adapter/config/*`: consider `make test-update-golden`.
