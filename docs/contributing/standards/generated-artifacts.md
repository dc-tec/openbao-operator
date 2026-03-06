# Generated Artifacts

This project contains several artifacts that are automatically generated from source code or templates.

!!! warning "Do Not Edit Manually"
    **Never** edit generated files directly. Your changes will be overwritten by the next build.
    Always edit the **Source** file and run the appropriate **Make Command**.

## Quick Reference

| If you modified... | Run this command | Verified by CI target |
| :--- | :--- | :--- |
| `api/v1alpha1/*.go` | `make manifests generate` | `verify-generated` |
| `api/v1alpha1/*.go` (API reference docs) | `make api-reference` | `verify-generated` |
| `dist/install.yaml` | `make helm-sync` | `verify-helm` |
| `.ast-grep/policy/architecture-boundaries.yml` | `make generate-ast-rules` | `verify-arch-policy` |
| `internal/adapter/config/*.go` | `make test-update-golden` | `test` (fails if mismatch) |
| **I don't know** | `make generate manifests api-reference helm-sync generate-ast-rules` | `verify-generated`, `verify-arch-policy` |

## Artifact details

### 1. Kubernetes CRDs & DeepCopy

Standard Kubebuilder artifacts generated from Go types.

- **Source:** `api/v1alpha1/*.go` (structs + kubebuilder markers)
- **Output:**
  - `config/crd/bases/` (CRD manifests)
  - `api/v1alpha1/zz_generated.deepcopy.go` (Go deepcopy methods)
- **Command:** `make manifests generate`

### 2. API Reference Docs

CRD API reference docs are generated from Go API types and comments.

- **Source:** `api/v1alpha1/*.go`
- **Output:**
  - `docs/reference/api.md`
- **Command:** `make api-reference`

### 3. Helm Chart Sync

We maintain a standalone Helm chart that must stay in sync with our core manifests.

- **Source:** `config/crd/bases/` + `config/policy/` + `config/rbac/`
- **Output:**
  - `charts/openbao-operator/crds/` (Synced CRDs)
  - `charts/openbao-operator/templates/admission/` (Synced admission policies)
  - `charts/openbao-operator/templates/rbac/` (Synced RBAC templates)
- **Command:** `make helm-sync`

### 4. Golden Test Files

We use "Golden Files" to verify complex HCL configuration generation reliability.

- **Source:** `internal/adapter/config/builder.go` logic changes
- **Output:** `internal/adapter/config/testdata/*.golden.hcl`
- **Command:** `make test-update-golden`

### 5. Ast-Grep Architecture Boundary Rules

Architecture boundary rules are generated from policy for CI enforcement.

- **Source:** `.ast-grep/policy/architecture-boundaries.yml`
- **Output:** `.ast-grep/rules/generated/architecture-boundary/*.yml`
- **Command:** `make generate-ast-rules`

## Troubleshooting

### CI Failure: "Diff found in generated files"

If `verify-generated`, `verify-helm`, or `verify-arch-policy` fails in CI, it means generated artifacts are out of date.

**Fix:**

1. Run the suggested command locally (e.g., `make manifests generate api-reference helm-sync generate-ast-rules`).
2. Verify you see changes in `config/`, `docs/reference/`, or `charts/`.
3. Commit and push those changes.
