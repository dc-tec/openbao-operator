# Generated Artifacts

This project contains several artifacts that are automatically generated from source code or templates.

!!! warning "Do Not Edit Manually"
    **Never** edit generated files directly. Your changes will be overwritten by the next build.
    Always edit the **Source** file and run the appropriate **Make Command**.

## Quick Reference

| If you modified... | Run this command | Verified by CI target |
| :--- | :--- | :--- |
| `api/v1alpha1/*.go` | `make manifests generate` | `verify-generated` |
| `internal/provisioner` | `make rbac-sync` | `verify-rbac-sync` |
| `dist/install.yaml` | `make helm-sync` | `verify-helm` |
| `internal/config/*.go` | `make test-update-golden` | `test` (fails if mismatch) |
| **I don't know** | `make generate manifests helm-sync` | `verify-generated` |

## Artifact details

### 1. Kubernetes CRDs & DeepCopy

Standard Kubebuilder artifacts generated from Go types.

- **Source:** `api/v1alpha1/*.go` (structs + kubebuilder markers)
- **Output:**
  - `config/crd/bases/` (CRD manifests)
  - `api/v1alpha1/zz_generated.deepcopy.go` (Go deepcopy methods)
- **Command:** `make manifests generate`

### 2. Helm Chart Sync

We maintain a standalone Helm chart that must stay in sync with our core manifests.

- **Source:** `config/crd/bases/` + `dist/install.yaml`
- **Output:**
  - `charts/openbao-operator/crds/` (Synced CRDs)
  - `charts/openbao-operator/files/install.yaml.tpl` (Synced installer)
- **Command:** `make helm-sync`

### 3. Provisioner RBAC

The Provisioner Delegate's permissions are strictly defined in Go code to ensure security.

- **Source:** `internal/provisioner/rbac.go` (Go struct definitions)
- **Output:** `config/rbac/provisioner_delegate_clusterrole.yaml`
- **Command:** `make rbac-sync`

### 4. Golden Test Files

We use "Golden Files" to verify complex HCL configuration generation reliability.

- **Source:** `internal/config/builder.go` logic changes
- **Output:** `internal/config/testdata/*.golden.hcl`
- **Command:** `make test-update-golden`

## Troubleshooting

### CI Failure: "Diff found in generated files"

If the `verify-generated` or `verify-helm` job fails in CI, it means you forgot to check in the updated artifacts.

**Fix:**

1. Run the suggested command locally (e.g., `make manifests generate helm-sync`).
2. Verify you see changes in `config/` or `charts/`.
3. Commit and push those changes.
