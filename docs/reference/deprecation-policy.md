---
title: Deprecation Policy
description: Deprecation and API lifecycle policy for OpenBao Operator, including pre-1.0 compatibility expectations and migration requirements.
pageType: reference
journey: reference
---

<PageHeader
  title="Deprecation and API lifecycle policy"
  lede="How deprecations are announced, how removals happen, and what migration guidance must ship with a breaking or removing change."
/>

<DecisionTable
  kind="reference"
  title="Pre-GA lifecycle rules"
  columns={['Change type', 'Current contract', 'Contributor or maintainer expectation']}
  rows={[
    {
      cells: [
        'Minor releases (`0.Y.0`)',
        'May include breaking API or behavior changes while the project remains pre-GA.',
        'Document the break clearly and ship a migration path.',
      ],
      emphasis: 'caution',
    },
    {
      cells: [
        'Patch releases (`0.Y.Z`)',
        'Should avoid intentional breaking changes.',
        'Reserved for compatible changes or changes required for safety and integrity.',
      ],
    },
    {
      cells: [
        'Security or data-integrity fixes',
        'May force urgent behavior changes earlier than the normal removal timeline.',
        'Call the exception out explicitly in release notes and migration guidance.',
      ],
    },
  ]}
/>

## Scope

This policy applies to:

- CRD API versions (`openbao.org/*`)
- CRD fields (`spec`, `status`)
- user-visible defaults and behavior contracts
- operator installation and upgrade workflows

## Deprecation process

When a field or behavior is deprecated, the project aims to do all of the following in the same release:

1. Mark the deprecation in API comments, which feed the generated API docs.
2. Document the deprecation in [API Reference](api.md) and release notes.
3. Provide a migration path and a concrete replacement example.

## Removal policy

For pre-1.0 releases:

- removals are expected in minor releases, not patch releases
- deprecated fields should remain available for at least one minor release when feasible
- urgent safety or security concerns may force earlier removal, but only with explicit release notes

<DecisionTable
  kind="reference"
  title="Migration requirements for breaking changes"
  columns={['Required output', 'Why it exists']}
  rows={[
    {
      cells: ['A migration section in release notes or changelog', 'Operators need a single place to see what changed and in what order to act.'],
      emphasis: 'recommended',
    },
    {
      cells: ['Clear before-and-after manifests', 'Schema changes are easier to adopt when the new target shape is concrete.'],
    },
    {
      cells: ['Upgrade sequencing notes', 'Some changes are safe only when CRDs, operator version, and workload rollout happen in the right order.'],
    },
  ]}
/>

## Kubernetes versioning mechanics

The project currently serves a single CRD API version, `v1alpha1`. When additional CRD versions are introduced, the project will use Kubernetes-native lifecycle controls such as:

- `served` and `storage`
- `deprecated: true`
- `deprecationWarning`

<NextActions
  title="Related lifecycle references"
  items={[
    {
      label: 'Compatibility matrix',
      description: 'Platforms and versions that remain in scope.',
      docId: 'reference/compatibility',
    },
    {
      label: 'Upgrade compatibility',
      description: 'Sequencing and rollback guidance for operator upgrades.',
      docId: 'reference/operator-upgrade-compatibility',
    },
    {
      label: 'Release management',
      description: 'Maintainer workflow for shipping a release that contains the deprecation.',
      to: '/contribute/release-management',
    },
  ]}
/>
