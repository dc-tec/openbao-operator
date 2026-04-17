---
title: Support Policy
description: Support and maintenance policy for OpenBao Operator covering pre-GA posture, release channels, and best-effort latest-line support.
pageType: reference
journey: reference
---

<PageHeader
  title="Support and maintenance policy"
  lede="Release lines that receive best-effort maintenance attention, channel differences, and issue-triage expectations."
/>

<Callout type="note" title="Current support window">

The project provides best-effort support for the latest stable release line.

</Callout>

<DecisionTable
  kind="reference"
  title="Release channels"
  columns={['Channel', 'What it is for', 'Support stance']}
  rows={[
    {
      cells: ['Stable (`X.Y.Z`)', 'Real deployments and the main production line.', 'Best-effort support on the latest stable line.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Prerelease (`-rc`, `-beta`, `-alpha`)', 'Evaluation before the next stable release.', 'No expanded support window; use for testing and early adoption only.'],
    },
    {
      cells: ['Edge (`main` snapshots)', 'Continuous validation and integration signal.', 'Evaluation and validation channel only.'],
      emphasis: 'caution',
    },
    {
      cells: ['Nightly', 'Scheduled validation snapshots.', 'Evaluation and validation channel only.'],
      emphasis: 'caution',
    },
  ]}
/>

## Pre-GA release contract

OpenBao Operator is still pre-GA:

- the served CRD API is `openbao.org/v1alpha1`
- minor releases may introduce breaking API or behavior changes
- support is best-effort and focused on the latest stable release line

## Validation versus support

- [Compatibility Matrix](compatibility.md) defines what is explicitly validated in CI.
- This policy defines what receives best-effort maintenance attention.
- `Recommended for production` means the documented hardened operating path, not a promise of long-lived pre-GA API stability.

## Security fixes

Security fixes follow [SECURITY.md](https://github.com/dc-tec/openbao-operator/blob/main/SECURITY.md):

- security fixes are provided for the latest released version
- vulnerabilities should be reported through GitHub Security Advisories

<DecisionTable
  kind="reference"
  title="Operator expectations for production use"
  columns={['Expectation', 'Why it matters']}
  rows={[
    {
      cells: ['Pin explicit operator and chart versions', 'Floating channels make support and rollback reasoning weaker.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Stay close to the latest stable release', 'The project focuses its maintenance effort on that line.'],
    },
    {
      cells: ['Use the `Hardened` profile with admission enforcement enabled', 'This is the documented production posture behind most guidance.'],
    },
    {
      cells: ['Validate upgrades in staging', 'Best-effort support does not remove the need for environment-specific rehearsal.'],
    },
    {
      cells: ['Avoid prerelease, edge, and nightly for production', 'These channels are designed for evaluation and validation, not supported production drift.'],
      emphasis: 'caution',
    },
  ]}
/>

<NextActions
  title="Related support references"
  items={[
    {
      label: 'Release policy',
      description: 'Public cadence and release-gate rules.',
      docId: 'reference/release-policy',
    },
    {
      label: 'Compatibility matrix',
      description: 'Platforms and versions exercised by CI.',
      docId: 'reference/compatibility',
    },
    {
      label: 'Known limitations',
      description: 'Current caveats and explicit non-goals behind unsupported behavior.',
      docId: 'reference/known-limitations',
    },
    {
      label: 'Release management',
      description: 'Maintainer release workflow behind these support expectations.',
      to: '/contribute/release-management',
    },
  ]}
/>
