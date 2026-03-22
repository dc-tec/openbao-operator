---
description: Distribution strategy for OpenBao Operator.
---

# Distribution

This project uses an Artifact Hub-first distribution strategy and defers public OperatorHub publication until post-pre-GA maturity gates are met.

## Current Strategy

1. Publish Helm OCI chart releases to `ghcr.io/dc-tec/charts/openbao-operator`.
2. Index those releases in Artifact Hub for discovery.
3. Keep OLM bundle assets in-repo and CI-validated, but do not submit publicly yet.

## Artifact Hub (Helm OCI)

### Repository Registration

When adding the repository in Artifact Hub:

1. Set `Kind` to `Helm charts`.
2. Set `URL` to `oci://ghcr.io/dc-tec/charts/openbao-operator`.

### Chart Metadata (Annotations)

`charts/openbao-operator/Chart.yaml` should include Artifact Hub annotations for discoverability and operator UX:

- `artifacthub.io/category`
- `artifacthub.io/license`
- `artifacthub.io/operator`
- `artifacthub.io/operatorCapabilities`
- `artifacthub.io/prerelease` (set to `"true"` for prereleases)
- `artifacthub.io/containsSecurityUpdates` (set to `"true"` when applicable for a release)
- `artifacthub.io/images` (explicit image list for reliable Artifact Hub security scanning)
- `artifacthub.io/crds` (operator CRD cards in Artifact Hub)
- `artifacthub.io/crdsExamples` (example CR manifests per CRD)
- `artifacthub.io/maintainers`
- `artifacthub.io/links`

### Verified Publisher / Ownership (OCI)

For OCI repositories, `artifacthub-repo.yml` is pushed to the same OCI repo path using the special tag `artifacthub.io`.

```bash
oras push \
  ghcr.io/dc-tec/charts/openbao-operator:artifacthub.io \
  --config /dev/null:application/vnd.cncf.artifacthub.config.v1+yaml \
  artifacthub-repo.yml:application/vnd.cncf.artifacthub.repository-metadata.layer.v1.yaml
```

`artifacthub-repo.yml` must include at least:

- `repositoryID` for verified publisher flow
- `owners` for ownership claim flow

### Release Verification

After each release:

1. Confirm chart version appears in Artifact Hub.
2. Confirm install instructions resolve for Helm OCI.
3. Confirm package metadata renders (links, maintainers, capabilities, prerelease flag).
4. Confirm verified publisher badge state is as expected.

## References

- [Artifact Hub Helm annotations](https://artifacthub.io/docs/topics/annotations/helm/)
- [Artifact Hub Helm chart repositories](https://artifacthub.io/docs/topics/repositories/helm-charts/)
- [Artifact Hub repositories (verified publisher / ownership)](https://artifacthub.io/docs/topics/repositories/)

