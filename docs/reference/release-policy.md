---
title: Release Policy
description: Public release cadence and gating policy for OpenBao Operator, including edge, nightly, prerelease, stable, and patch release rules.
pageType: reference
journey: reference
---

<PageHeader
  title="Release channels and cadence"
  lede="Public cadence, release channels, and gating rules for OpenBao Operator."
/>

<Callout type="important" title="Best-effort cadence">

The project aims for one stable release every 4 weeks. If the stable release gates are not green, the release window is skipped rather than forced.

</Callout>

<DecisionTable
  kind="reference"
  title="Release channels"
  columns={['Channel', 'Target cadence or trigger', 'What it is for', 'Public expectation']}
  rows={[
    {
      cells: [
        'Stable (`X.Y.Z`)',
        'Target every 4 weeks when the stable release gates are green.',
        'The supported production line.',
        'Best-effort support on the latest stable line only.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Patch release',
        'Out-of-band when there is a regression, security fix, or sharp-edge fix worth shipping sooner.',
        'Fast correction without waiting for the monthly train.',
        'Keep the scope tight and ship the next patch version directly.',
      ],
    },
    {
      cells: [
        'Prerelease (`-rc.N`, `-beta.N`, `-alpha.N`)',
        'Used for minor releases and for any release with meaningful runtime, security, or lifecycle changes.',
        'Soak the next stable release before final cut.',
        'Use for evaluation and staging, not as a supported production channel.',
      ],
    },
    {
      cells: [
        'Nightly',
        'Every successful nightly validation run.',
        'Scheduled lifecycle validation and drift detection.',
        'Not a production support channel.',
      ],
      emphasis: 'caution',
    },
    {
      cells: [
        'Edge (`main` snapshots)',
        'Every green merge to `main`.',
        'Continuous integration signal and early validation.',
        'Not a production support channel.',
      ],
      emphasis: 'caution',
    },
  ]}
/>

## Monthly release model

<DecisionTable
  kind="reference"
  title="Default release rhythm"
  columns={['Window', 'Maintainer focus', 'Expected output']}
  rows={[
    {
      cells: [
        'Weeks 1-3',
        'Normal development on `main`.',
        'Features, fixes, and continuous edge validation.',
      ],
    },
    {
      cells: [
        'Week 4 hardening window',
        'Avoid broad refactors. Focus on docs, regressions, generated artifacts, and upgrade, backup, and restore confidence.',
        'Prepare the next stable release candidate or skip the window if the gates are not ready.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'RC soak',
        'Use a release candidate for minor releases and for non-trivial runtime, security, or lifecycle changes.',
        'Gather soak evidence before the stable cut.',
      ],
    },
  ]}
/>

## Stable release gates

A stable release should meet all of the following:

- clean PR CI on the release branch or release PR
- full E2E release evidence from the release PR or a maintainer-triggered CI run on the release branch
- docs and compatibility matrix reviewed and current; new stable release lines have a committed `X.Y.0` docs snapshot
- nightly validation reviewed for regressions; known flaky nightlies are tracked, but they do not block by themselves when release E2E evidence is clean
- performance signal reviewed; block only on confirmed product regressions or an explicit maintainer decision while the harness is being calibrated
- no known upgrade, backup, or restore regressions
- no open release-blocker issues

<Callout type="note" title="Release window skips">

If a monthly release window is not ready, skip it and say so. Predictability matters more than pushing a release that has not met the public gates.

</Callout>

## Patch release policy

- Do not wait for the monthly stable train when the fix materially improves safety, correctness, or operator usability.
- Use the next patch version directly.
- Keep patch releases narrowly scoped and avoid unrelated refactors.

## Current pre-beta support stance

Until the project reaches beta, the support window stays on the latest stable line only. Older stable lines may stop receiving fixes as soon as a newer stable release ships.

<NextActions
  title="Related release references"
  items={[
    {
      label: 'Support policy',
      description: 'Maintenance contract for releases after they ship.',
      docId: 'reference/support-policy',
    },
    {
      label: 'Compatibility matrix',
      description: 'Kubernetes and OpenBao versions exercised by CI.',
      docId: 'reference/compatibility',
    },
    {
      label: 'Release management',
      description: 'Maintainer workflow for executing a release.',
      to: '/contribute/release-management',
    },
  ]}
/>
