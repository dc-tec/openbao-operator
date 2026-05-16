---
title: Distribution
description: Distribution model for OpenBao Operator, including Artifact Hub, GHCR Helm OCI publication, verified ownership metadata, and intentionally deferred public OperatorHub publication.
pageType: concept
journey: contribute
---

<PageHeader
  title="Current distribution model"
  lede="OpenBao Operator uses an Artifact Hub-first distribution model. OCI Helm chart releases are published to GHCR, indexed in Artifact Hub for discovery, and OLM bundle assets remain in-repo and CI-validated rather than being part of the current supported publication surface."
/>

<DecisionTable
  title="Distribution surfaces"
  columns={["Surface", "Published now", "Current role"]}
  rows={[
    {
      cells: [
        "GHCR OCI Helm chart",
        "Yes",
        "Canonical chart publication target for released versions.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Artifact Hub package entry",
        "Yes",
        "Discovery and installation entry point for the published chart.",
      ],
    },
    {
      cells: [
        "GitHub Release assets",
        "Yes",
        "Installer manifests, CRDs, checksums, SBOMs, and provenance metadata.",
      ],
    },
    {
      cells: [
        "OLM bundle assets",
        "Kept in-repo and CI-validated",
        "Preparation surface only; not yet a public distribution contract.",
      ],
    },
    {
      cells: [
        "Public OperatorHub submission",
        "No",
        "Explicitly deferred until post-pre-GA maturity and support posture are ready.",
      ],
      emphasis: "caution",
    },
  ]}
/>

<Callout type="note" title="Artifact Hub first">

The project currently treats Artifact Hub plus GHCR Helm OCI publication as the supported public discovery model. Do not add new public distribution expectations without updating this policy and the release flow that backs it.

</Callout>

<Callout type="note" title="Installer manifests are channel artifacts">

`dist/install.yaml` and `dist/crds.yaml` are local build outputs, not source-controlled distribution assets. Release, edge, and nightly workflows generate fresh manifests from `config/` with the target image and operator version, then publish those generated files through the relevant channel.

</Callout>

<DecisionTable
  title="Artifact Hub metadata expectations"
  columns={["Metadata area", "What must be present"]}
  rows={[
    {
      cells: [
        "Chart annotations",
        "`artifacthub.io/category`, `license`, `operator`, `operatorCapabilities`, release changes, image metadata, CRD cards/examples, maintainers, and useful links.",
      ],
    },
    {
      cells: [
        "Prerelease and security state",
        "`artifacthub.io/prerelease` for prereleases, `artifacthub.io/changes` from the release-please changelog, and `artifacthub.io/containsSecurityUpdates` when applicable for the release.",
      ],
    },
    {
      cells: [
        "Verified ownership",
        "`artifacthub-repo.yml` with repository ID or ownership information published to the chart OCI path under the `artifacthub.io` tag.",
      ],
      emphasis: "recommended",
    },
  ]}
/>

<CommandBlock
  language="bash"
  label="apply"
  title="Push Artifact Hub repository metadata for the OCI chart"
  code={`oras push \\
  ghcr.io/dc-tec/charts/openbao-operator:artifacthub.io \\
  --config /dev/null:application/vnd.cncf.artifacthub.config.v1+yaml \\
  artifacthub-repo.yml:application/vnd.cncf.artifacthub.repository-metadata.layer.v1.yaml`}
>
  Use this only when `artifacthub-repo.yml` changes or ownership metadata must be refreshed for the chart repository.
</CommandBlock>

<NextActions
  title="After distribution review"
  items={[
    {
      label: "Release management",
      description: "Go back to the release workflow when you are publishing a version rather than changing distribution policy.",
      to: "/contribute/release-management",
    },
    {
      label: "Supply chain security",
      description: "Open the policy side when distribution changes affect provenance, signing, or verification expectations.",
      to: "/contribute/supply-chain-security",
    },
    {
      label: "Dependency license policy",
      description: "Use the governance policy if the published artifact surface changes what licenses are considered shipped.",
      to: "/contribute/dependency-licenses",
    },
  ]}
/>
