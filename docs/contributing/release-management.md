# Release Management

We follow a strict "Build Once, Promote Everywhere" philosophy. Releases are automated, signed, and provenanced.

!!! note "Version format"
    Git tags and Helm chart versions use SemVer **without** a leading `v` (for example: `0.1.0`, `0.2.0-rc.1`).

## 0. Channels

We publish multiple channels:

- **Stable / SemVer**: `MAJOR.MINOR.PATCH` (and prereleases like `X.Y.Z-rc.1`, `X.Y.Z-beta.1`, `X.Y.Z-alpha.1`). This is the only channel that publishes OCI Helm charts and GitHub Release assets.
- **Edge** (main): published automatically after CI passes on `main` (tags: `edge`, `edge-<shortsha>`), with signed manifests published to GitHub Pages under `/edge/<shortsha>/` and `/edge/latest/`. No OCI Helm chart publication. Edge is for pre-release validation and is not supported for production.
- **Nightly**: published automatically after nightly E2E passes (tags: `nightly`, `nightly-YYYYMMDD`, `nightly-YYYYMMDD-<shortsha>`), and published as mutable manifests on GitHub Pages under `/nightly/`. No OCI Helm chart publication.

## 0.1 Release-Please (Versioning + Release PRs)

We use **release-please** as the source of truth for:

- Open/maintain a Release PR on `main` based on Conventional Commits.
- Update `CHANGELOG.md` and bump `charts/openbao-operator/Chart.yaml` (chart `version` and `appVersion`).
- Create the `X.Y.Z` tag and a draft GitHub Release when the Release PR is merged.

!!! important "Required token"
    `release-please` must use a non-default token, otherwise the resulting tag/release may not trigger downstream GitHub Actions workflows.

    Recommended: configure a GitHub App (for example `openbao-operator-release`) and use its installation token (no server required; no webhook/callback needed). Store:

    - `OPENBAO_OPERATOR_RELEASE_APP_ID`
    - `OPENBAO_OPERATOR_RELEASE_PRIVATE_KEY`

    Fallback: if you prefer a PAT, configure a repository secret named `RELEASE_PLEASE_TOKEN` with permissions to:
    - Create/update PRs
    - Create tags and releases
    and update `.github/workflows/release-please.yml` to use it.

!!! tip "Keep release-please PRs mergable"
    If you enforce "required approval" rules, avoid using a PAT for your own user as `RELEASE_PLEASE_TOKEN`.
    GitHub often excludes self-approvals from required reviews. Prefer a bot account or GitHub App.

## 0.2 Automation Workflows

=== "Stable / prerelease"

    - Versioning + release notes: `.github/workflows/release-please.yml`
    - Build + gates + artifacts: `.github/workflows/release.yml`

=== "Edge (main)"

    - After CI success on `main`, `.github/workflows/publish-edge.yml` publishes:
        - Images: `:edge` and `:edge-<shortsha>`
        - Manifests to GitHub Pages:
          - immutable per-commit: `/edge/<shortsha>/install.yaml` and `/edge/<shortsha>/crds.yaml`
          - moving pointer: `/edge/latest/install.yaml` and `/edge/latest/crds.yaml`
          - plus checksums, checksums bundle, and metadata in both paths
        - No Helm chart publication (release-only)

=== "Nightly"

    - After nightly E2E success, `.github/workflows/publish-nightly.yml` publishes:
        - Images: `:nightly`, `:nightly-YYYYMMDD`, `:nightly-YYYYMMDD-<shortsha>`
        - Manifests to GitHub Pages: `/nightly/install.yaml`, `/nightly/crds.yaml` (+ checksums, checksums bundle, metadata)
        - No Helm chart publication (release-only)

## 1. Stable/Prerelease Release Flow

`release-please` is responsible for *versioning and release notes*. The `Release` workflow is responsible for *building, gating, and publishing artifacts*.

!!! note "Chart version validation contract"
    The `Release` workflow validates that the git tag (`X.Y.Z` or prerelease tag) matches both `charts/openbao-operator/Chart.yaml` fields:

    - `version`
    - `appVersion`

    If either value differs from the tag, the release fails fast in `prepare`.
    The workflow does **not** override chart version fields at package time; Helm packaging uses the committed `Chart.yaml` values.

Our pipeline ensures that the artifacts we test in E2E are the *exact* same bits that are published (bit-for-bit identical).

```mermaid
graph TD
    RP[Merge release-please PR] --> GitTag[Git tag: X.Y.Z]

    GitTag --> Prepare[Prepare variables]

    subgraph Build [Build Once]
        Img[Build + push images :build-SHA]
    end

    subgraph Gate [Gates]
        Vuln[govulncheck]
        Scan[Trivy image scan]
        Test[E2E matrix]
        Perf[Performance gate]
    end

    subgraph Promote ["Promote + Publish (No Rebuild)"]
        Retag["Promote tags by digest (:X.Y.Z)"]
        Sign[Cosign sign + attest]
        Chart[Package + push OCI Helm chart]
        Manifests[Generate install.yaml + crds.yaml]
        SBOM[Generate SBOMs + checksums]
    end

    subgraph Publish [GitHub Release + Docs]
        GH[Create/Update GitHub Release]
        Assets[Upload dist/* assets]
        Docs[Deploy versioned docs]
    end

    Prepare --> Img
    Img --> Gate
    Gate --> Promote
    Promote --> Publish

    classDef process fill:transparent,stroke:#9333ea,stroke-width:2px,color:#fff;
    classDef read fill:transparent,stroke:#60a5fa,stroke-width:2px,color:#fff;
    classDef write fill:transparent,stroke:#22c55e,stroke-width:2px,color:#fff;
    classDef security fill:transparent,stroke:#dc2626,stroke-width:2px,color:#fff;
    classDef git fill:transparent,stroke:#f472b6,stroke-width:2px,color:#fff;
    
    class RP,Prepare process;
    class GitTag git;
    class Img,Retag,Sign,Chart,Manifests,SBOM,GH,Assets,Docs write;
    class Vuln,Scan security;
    class Test,Perf read;
```

## 2. Triggers

=== "Release-please"

    - Merge the release-please PR on `main`.
    - Wait for the tag to be created and for the `Release` workflow to publish artifacts.

!!! note "Policy: release only when ready"
    The repository includes an gate (`.github/workflows/release-pr-gate.yml`) intended for the release-please PR:

    - Require a label (default: `release:ready`)
    - Require an explicit approval from the designated release manager (default: `@dc-tec`)

!!! note "Manual releases"
    Stable/prerelease releases should be created by merging the release-please PR.
    For out-of-band release, prefer cutting a dedicated PR on `main` and letting release-please generate the tag/release.

!!! note "Prereleases"
    For `-alpha.*`, `-beta.*`, and `-rc.*` tags, the GitHub Release is marked as a prerelease and docs are published without moving the `latest` alias.

## 2.1 Cutting Beta/RC Releases (Release-As)

By default, `release-please` determines the next version bump from Conventional Commits:

- `fix:` -> patch bump
- `feat:` -> minor bump
- `feat!:` (or `BREAKING CHANGE:`) -> major bump

When a ad-hoc prerelease is needed (for example `0.2.0-beta.1` or `0.2.0-rc.1`), override the target version using `Release-As:` in a commit that lands on `main`. `release-please` will update the Release PR to that exact version; merge it when you are ready to publish the prerelease.

=== "Override via empty commit"

    ```sh
    git commit --allow-empty -m "chore: release 0.2.0-beta.1" -m "Release-As: 0.2.0-beta.1"
    git push
    ```

=== "Override via PR merge message"

    Add the override line to the squash/merge commit message:

    ```text
    chore: prepare 0.2.0-beta.1

    Release-As: 0.2.0-beta.1
    ```

!!! note "Iterating prereleases"
    Repeat the same process for `0.2.0-beta.2`, then `0.2.0-rc.1`, and finally `0.2.0`.

## 3. Published Artifacts (Stable/Prerelease Only)

The stable/prerelease release produces the following artifacts:

=== "GitHub Release assets"

    - `install.yaml` (digest-pinned installer manifest)
    - `crds.yaml` (CRDs only)
    - `checksums.txt` (sha256 of `install.yaml`, `crds.yaml`, and SBOMs)
    - `checksums.txt.bundle` (keyless Sigstore bundle for `checksums.txt`)
    - `sbom-openbao-operator.spdx.json`
    - `sbom-openbao-init.spdx.json`
    - `sbom-openbao-backup.spdx.json`
    - `sbom-openbao-upgrade.spdx.json`

=== "Container images"

    - `ghcr.io/<OWNER>/openbao-operator:X.Y.Z`
    - `ghcr.io/<OWNER>/openbao-init:X.Y.Z`
    - `ghcr.io/<OWNER>/openbao-backup:X.Y.Z`
    - `ghcr.io/<OWNER>/openbao-upgrade:X.Y.Z`

=== "Helm chart (OCI)"

    - `ghcr.io/<OWNER>/charts/openbao-operator:X.Y.Z`

## 4. Release Checklist

For Release Managers.

### Pre-Flight Checks

- [ ] **Changelog**: Ensure the release-please PR looks correct (changelog entries and version bumps).
- [ ] **Docs**: Ensure documentation is consistent with new features.
- [ ] **Compatibility**: Verify `docs/reference/compatibility.md` covers the supported versions.
- [ ] **Clean CI**: Ensure the latest commit on main is green.
- [ ] **Performance Gate**: Run the **Performance Baseline Capture** workflow on `main` (or release branch), then ensure `hack/perf/baseline/kind-v1.34.3-baseline.json` and `hack/perf/thresholds/kind-v1.34.3.yaml` in-repo match captured evidence and `make verify-perf` passes.

### Post-Release

- [ ] **Verify**: Check that the GitHub Release exists and assets are valid.
- [ ] **Artifact Hub**: Confirm chart package metadata is visible and install instructions resolve.
- [ ] **Artifact Hub Metadata (OCI)**: If `artifacthub-repo.yml` changed, push updated metadata artifact (`:artifacthub.io` tag) to GHCR.
- [ ] **Backlink Pack**: Publish and record:
  - GitHub release notes URL
  - Artifact Hub package URL
  - OpenBao docs/community URL
  - GitHub Discussions announcement URL
- [ ] **Announce**: Post in relevant community channels.

### Artifact Hub Metadata Sync (OCI)

For OCI Helm repositories, `artifacthub-repo.yml` is not served over HTTP. Push it as an OCI artifact to the same chart repository path:

```bash
oras push \
  ghcr.io/dc-tec/charts/openbao-operator:artifacthub.io \
  --config /dev/null:application/vnd.cncf.artifacthub.config.v1+yaml \
  artifacthub-repo.yml:application/vnd.cncf.artifacthub.repository-metadata.layer.v1.yaml
```

## 5. Verifying Artifacts

All artifacts are signed using Sigstore (Keyless).

=== ":material-check-decagram: Verify Image Signature"
    Using `cosign` to verify the image was built by our release workflow.

    ```sh
    cosign verify \
      --new-bundle-format=true \
      --certificate-identity-regexp "https://github.com/dc-tec/openbao-operator/.github/workflows/release.yml" \
      --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
      ghcr.io/dc-tec/openbao-operator:0.1.0
    ```

=== ":material-file-certificate: Verify Attestation"
    Using GitHub CLI to verify build provenance.

    ```sh
    gh attestation verify \
      oci://ghcr.io/dc-tec/openbao-operator:0.1.0 \
      --owner dc-tec
    ```

=== ":material-chart-bubble: Verify Helm Chart"
    Verify the OCI Helm Chart signature by digest.

    ```sh
    # Resolve the chart digest (example uses crane)
    crane digest ghcr.io/dc-tec/charts/openbao-operator:0.1.0

    cosign verify \
      --new-bundle-format=true \
      --certificate-identity-regexp "https://github.com/dc-tec/openbao-operator/.github/workflows/release.yml" \
      --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
      ghcr.io/dc-tec/charts/openbao-operator@sha256:...
    ```

=== ":material-file-lock: Verify Release Checksums"
    Verify `checksums.txt` using the Sigstore bundle uploaded to the GitHub Release.

    ```sh
    cosign verify-blob \
      --new-bundle-format=true \
      --bundle dist/checksums.txt.bundle \
      --certificate-identity-regexp "https://github.com/dc-tec/openbao-operator/.github/workflows/release.yml" \
      --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
      dist/checksums.txt
    ```
