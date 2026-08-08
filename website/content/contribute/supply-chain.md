---
title: Protect the software supply chain
description: Apply dependency, provenance, reproducibility, distribution, and governance controls from design through publication.
eyebrow: Contribute
weight: 5
verifiedBy:
  - .github/workflows/ci.yml
  - .github/workflows/release.yml
  - .github/workflows/publish-edge.yml
  - .github/workflows/publish-nightly.yml
  - .github/workflows/reusable-channel-hardening.yml
  - Makefile
---

Treat planning, implementation, verification, publication, and operational feedback as one lifecycle. Release controls cannot compensate for an unreviewable design or an unverified implementation.

## Build once and promote the same subject

Build immutable artifacts, verify their provenance and reproducibility, then promote those subjects by digest. Do not rebuild during publication and claim the new bytes are the object that passed earlier checks.

| Channel | Trust posture | Published surface |
| --- | --- | --- |
| Pull-request CI | Validation only; no publication | Test and policy evidence |
| Edge and nightly | Provenance and byte-reproducibility gates | Channel manifests, checksums, and provenance metadata |
| Prerelease and stable | Provenance, reproducibility, signing, and release evidence | Images, chart, manifests, CRDs, checksums, SBOMs, notes, and attestations |

Use vendored Go dependencies, pinned workflow actions and build inputs, deterministic artifact generation, identity-constrained signing, and retained release evidence. A reproducibility failure indicates changed or nondeterministic inputs; diagnose it before publication.

{{< callout type="warning" title="Single-maintainer constraint" >}}
Automation, pinned identities, and retained evidence improve assurance, but they do not create genuine two-person release separation. Record this limitation instead of treating automated approval as independent human review.
{{< /callout >}}

## Enforce dependency policy

The blocking license gate covers the shipped controller, backup, upgrade, and provisioner binaries in vendored mode.

| License class | Policy |
| --- | --- |
| Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC, MIT, Unicode-DFS-2016 | Allowed with required notices |
| MPL-2.0 | Allowed with explicit file-level copyleft handling and review |
| Strong copyleft, source-available, field-of-use restricted, unknown, or unrecognized | Do not ship |

For a new MPL-2.0 dependency, preserve notices, avoid casual vendored patches, retain source for redistributed modified files when required, and call it out in the pull request. Treat allowlist changes as maintainer policy changes and update the machine-enforced configuration with the prose.

{{< command label="verify" title="Verify the shipped dependency graph" >}}
make verify-vendor
make license-check
make license-report
{{< /command >}}

GitHub dependency review catches newly introduced package vulnerabilities. The vendored full-tree gate and maintainer review remain authoritative for the license policy of shipped binaries.

## Keep distribution claims narrow

The supported public distribution path is the OCI Helm chart in GHCR, indexed by Artifact Hub, plus GitHub Release assets. OLM bundle assets remain repository-tested preparation material; public OperatorHub publication is not a current support contract.

Installer manifests under `dist/` are generated channel artifacts, not hand-maintained source. Artifact Hub metadata must describe the chart, changes, images, CRDs, maintainers, links, prerelease state, and security-update state. Repository ownership metadata is published separately for the OCI chart location.

## Escalate trust failures

Stop publication when signer identity, workflow identity, provenance, checksums, or rebuild bytes do not match. Preserve evidence and use the [supply-chain incident runbook]({{< relref "/contribute/incident-response.md" >}}) before rotating or reissuing anything.

Continue with [release management]({{< relref "/contribute/release.md" >}}).
