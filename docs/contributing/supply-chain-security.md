---
description: Supply-chain policy for release governance, provenance verification, and signoff evidence for OpenBao Operator stable/prerelease releases.
---

# Supply Chain Security

!!! note "Policy intent"
    Use this page as the source of truth for release supply-chain requirements and evidence.

## 1. Compliance Language

!!! success "Approved claim language"
    Use the following statements in release signoff and public documentation:

    1. OpenBao Operator release pipeline targets SLSA Build L3 controls for stable/prerelease releases.
    2. OpenBao Operator implements additional L4-like hardening controls where feasible.
    3. This is not a formal SLSA Build L4 conformance claim.

!!! warning "Formal Build L4 claim"
    Do not claim formal SLSA Build L4 conformance for this repository.

## 2. Scope (Wave 1)

!!! note "Wave 1 boundary"
    Formal scope is stable/prerelease releases only through `.github/workflows/release.yml`.

!!! tip "Channel hardening parity"
    Edge and nightly channels run the same strict build/provenance/byte-repro gates through `.github/workflows/reusable-channel-hardening.yml`.
    They are enforced release/build hardening controls, but are not part of the formal stable/prerelease claim boundary.

Artifacts in scope:

=== "Images"

    - `ghcr.io/<owner>/openbao-operator`
    - `ghcr.io/<owner>/openbao-init`
    - `ghcr.io/<owner>/openbao-backup`
    - `ghcr.io/<owner>/openbao-upgrade`

=== "Helm chart"

    - `ghcr.io/<owner>/charts/openbao-operator`

=== "Release files"

    - `dist/install.yaml`
    - `dist/crds.yaml`
    - `dist/checksums.txt`
    - `dist/checksums.txt.bundle`
    - `dist/sbom-*.spdx.json`
    - `dist/provenance-index.json`

## 3. Governance Controls

!!! warning "Single-maintainer constraint"
    Operating mode currently uses one maintainer. Keep approvals at `0` for now, but continue enforcing PR-based changes and required checks.

Required repository controls:

1. Protect `main` with default branch ruleset and required checks for release confidence.
2. Protect semver release tags (`[0-9]*.[0-9]*.[0-9]*` and prerelease variants) against deletion/retarget.
3. Enforce CODEOWNERS coverage for workflows, Dockerfiles, chart, and release/security docs.
4. Keep direct pushes blocked through ruleset policy on protected refs.
5. Keep GitHub Actions pinned by SHA; move to explicit allowlist as follow-up hardening.

## 4. Build/Release Integrity Policy

Mandatory controls:

1. Build once, promote by digest: tags/channels are promoted from pre-built digests only.
2. Use reusable build workflow as provenance authority for image attestations.
3. Require blocking provenance and byte-level reproducibility gates before publish/promote.
4. Enforce vendored Go dependency resolution (`-mod=vendor`) for CI/release build and test paths.
5. Require chart digest and `checksums.txt` GitHub attestations with in-pipeline verification.
6. Publish `provenance-index.json` as machine-readable verification metadata for release/edge/nightly channels.
7. Pin base images and critical toolchain versions to reduce drift.
8. Collect reproducibility diagnostics with `.github/workflows/reproducibility.yml` (report-only for stable/prerelease tags).

## 5. Verification Policy

Verify all release artifact classes before install/use:

1. Image signatures and image build attestations.
2. Chart signature and chart attestation.
3. `checksums.txt` signature and attestation.

!!! tip "Verification commands"
    Use the exact command set from [Release Management](release-management.md#5-verifying-artifacts).

## 6. Release Signoff Evidence

Capture the following for every stable/prerelease release:

1. `Release` workflow run URL and run ID.
2. Successful `verify-provenance` gate evidence.
3. `gh attestation verify` outputs for one image, chart digest, and `checksums.txt`.
4. GitHub Release asset list including `provenance-index.json`.
5. Ruleset export evidence for default-branch and tag rulesets.

!!! note "Ruleset export commands"

    ```sh
    gh api repos/dc-tec/openbao-operator/rulesets > rulesets.json
    gh api repos/dc-tec/openbao-operator/rulesets/<RULESET_ID> > ruleset-<RULESET_ID>.json
    ```

## 7. Troubleshooting Matrix

| Failure | Likely Cause | Resolution |
| :--- | :--- | :--- |
| `gh attestation verify` returns 404 | Attestation not yet indexed | Retry with bounded backoff (workflow already does this). |
| `signer workflow mismatch` | Wrong `--signer-workflow` identity | Use exact workflow path expected by policy (`reusable-build.yml` for images, `release.yml` for chart/checksums). |
| `source ref mismatch` | Attestation built from different ref | Verify with exact `refs/tags/<version>` source ref. |
| `self-hosted runner denied` | Build ran on non-GitHub-hosted runner | Re-run release on GitHub-hosted runner pool only. |
| `checksums.txt attestation verify failed` | File changed after attestation | Regenerate checksums, re-attest, and ensure no post-attestation mutation. |
| `cosign verify-blob` fails | Wrong bundle or identity constraint | Download matching `checksums.txt` and bundle from same release; verify against `release.yml@refs/tags/<version>`. |
