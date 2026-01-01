# Release Management

This document describes the best-practice release process for the OpenBao Operator.

## Goals

- **Build once, promote**: release artifacts are produced once and promoted by digest (no rebuild at GA).
- **Release is gated**: full E2E must pass before publishing.
- **GHCR only**: all images and the OCI Helm chart are published to GitHub Container Registry.
- **Signed artifacts**: images and the OCI Helm chart are signed using keyless signing (GitHub OIDC + Sigstore).
- **Provenance attestations**: publish build provenance for each image digest (GitHub attestations).
- **No convenience tags**: only version tags (e.g. `v0.1.0`) and internal build tags (e.g. `build-<sha>`).

## Release Workflow (GitHub Actions)

Releases are created via `.github/workflows/release.yml` (manual `workflow_dispatch`).

### Inputs

- `version`: required semver with leading `v` (e.g. `v0.1.0`)
- `ref`: optional git ref to release (defaults to the workflow ref)

### What It Does

1. **Build images** (multi-arch) and push to GHCR with a build tag `build-<sha>`.
2. **Security gates**:
   - `govulncheck -test ./...`
   - Trivy image scan for HIGH/CRITICAL vulns (build-tag images)
3. **Config compatibility gate**: validate operator-generated HCL against the upstream OpenBao config parser for the supported OpenBao version range (`>=2.4.0 <2.5.0`).
4. **E2E gate**: full E2E suite on a Kubernetes version matrix (from `docs/reference/compatibility.md`).
5. **Promote by digest (no rebuild)**:
   - Retag images by digest to `:<version>` (e.g. `:v0.1.0`)
6. **Sign artifacts** (keyless):
   - Sign each image by digest
   - Package and push the OCI Helm chart to `oci://ghcr.io/<org>/charts`
   - Sign the OCI chart
7. **Publish release assets**:
   - Generate `dist/install.yaml` pinned to the operator image digest
   - Generate SPDX JSON SBOMs for each published image and attach them to the GitHub Release
   - Generate `dist/checksums.txt`
8. **Create git tag**: annotated `vX.Y.Z` tag is pushed.
9. **Create GitHub Release**: release notes are generated and assets are attached.

## Artifact Locations

### Images

- `ghcr.io/<org>/openbao-operator:<version>`
- `ghcr.io/<org>/openbao-operator-sentinel:<version>`
- `ghcr.io/<org>/openbao-config-init:<version>`
- `ghcr.io/<org>/openbao-backup-executor:<version>`
- `ghcr.io/<org>/openbao-upgrade-executor:<version>`

### Helm (OCI)

- `oci://ghcr.io/<org>/charts/openbao-operator:<chart-version>`
- Chart version is the semver **without** the leading `v` (e.g. `0.1.0`), while `appVersion` remains `v0.1.0`.

## Maintaining the Helm Chart

The Helm chart is maintained as a first-class install path, with a small “generated surface area”:

- **CRDs** are generated from Go types into `config/crd/bases/*.yaml` and synced into `charts/openbao-operator/crds/`.
- The chart’s installer template `charts/openbao-operator/files/install.yaml.tpl` is derived from `dist/install.yaml` with Helm templating applied.

To sync these inputs after changing API types or manifests:

```sh
make helm-sync
```

To validate that the chart inputs are up-to-date (CI gate):

```sh
make verify-helm
```

### Installer Manifest

- GitHub Release asset: `dist/install.yaml` (digest-pinned operator image)
- GitHub Release assets: `dist/sbom-*.spdx.json` (per-image SPDX JSON SBOMs)

## Installing From a Release

### Installer YAML

Download `install.yaml` from the GitHub Release and apply it:

```sh
kubectl apply -f install.yaml
```

### Helm (OCI)

Install into the expected default namespace:

```sh
helm install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
  --version 0.1.0 \
  --namespace openbao-operator-system \
  --create-namespace
```

To pin the operator image by digest:

```sh
helm install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
  --version 0.1.0 \
  --namespace openbao-operator-system \
  --create-namespace \
  --set image.repository=ghcr.io/dc-tec/openbao-operator \
  --set image.digest=sha256:...
```

### CRD upgrades and Helm

Helm does not reliably upgrade CRDs from the chart `crds/` directory on `helm upgrade`.
For releases that change CRDs, the recommended process is:

1. Apply updated CRDs from the GitHub Release assets (or from `config/crd/bases/`) first.
2. Then run `helm upgrade`.

## Operator Versioning Notes

- Kubernetes compatibility is documented in `docs/reference/compatibility.md`.
- CRD versioning (e.g., future `v1beta1` / `v1`) should be handled as an additive evolution with a documented migration path and upgrade notes per release.

## Sigstore Trusted Root Maintenance

Keyless signature verification depends on the Sigstore trusted root bundle vendored in `internal/security/trusted_root.json`.

- Manual refresh: `make update-trusted-root verify-trusted-root`
- Automated refresh PRs: `.github/workflows/trusted-root-refresh.yml` (scheduled quarterly)

## Verifying Provenance

The release workflow publishes GitHub build provenance attestations for each image digest (in addition to cosign signatures).

Verification options:

- GitHub CLI: `gh attestation verify`
- OCI referrers: compatible tools can fetch attestation referrers for `ghcr.io/<org>/<image>@sha256:<digest>`
