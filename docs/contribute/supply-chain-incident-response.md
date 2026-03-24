---
title: Supply-Chain Incident Response
description: Maintainer runbook for responding to CI, release, GHCR, or GitHub App supply-chain incidents in OpenBao Operator.
pageType: task
journey: contribute
---

<PageHero
  variant="compact"
  eyebrow="Contribute / Project Governance"
  title="Use this runbook when you need to freeze release automation fast and recover with evidence."
  lede="During a supply-chain incident, speed and containment matter more than elegance. This runbook is for freezing publication, suspending high-trust credentials, inspecting recent releases, and restoring trust in a controlled order."
  actions={[
    {label: "Open supply chain security", to: "/contribute/supply-chain-security", variant: "primary"},
    {label: "Open release management", to: "/contribute/release-management", variant: "secondary"},
  ]}
>
  <Checklist
    title="Use this page when you need to"
    items={[
      "contain a suspected compromise in GitHub Actions, GHCR, or release automation",
      "suspend one or both release GitHub Apps and rotate their private keys",
      "freeze semver tag creation while you assess the blast radius",
      "inspect recent workflow runs, release tags, draft releases, and promoted images",
    ]}
  />
</PageHero>

<DecisionTable
  title="Immediate containment order"
  columns={["Priority", "Action", "Why it comes first"]}
  rows={[
    {
      cells: [
        "1",
        "Disable GitHub Actions or restrict them to approved workflows only until the cause is understood.",
        "Stops further workflow-triggered token minting, package publication, or release mutation.",
      ],
      emphasis: "caution",
    },
    {
      cells: [
        "2",
        "Suspend `openbao-operator-release-pr` and `openbao-operator-release-tag` if release automation is in scope.",
        "Cuts off the highest-trust automation identities before new tags or draft releases can appear.",
      ],
    },
    {
      cells: [
        "3",
        "Freeze semver tag creation and updates through repository rulesets.",
        "Prevents a forged or replayed release tag from triggering the stable release pipeline.",
      ],
    },
    {
      cells: [
        "4",
        "Rotate app private keys and any affected repository secrets.",
        "Assumes any app key or workflow-level credential may already have leaked.",
      ],
    },
  ]}
/>

<CommandBlock
  language="bash"
  label="inspect"
  title="List recent high-trust workflow runs"
  code={`gh run list \
  --repo dc-tec/openbao-operator \
  --limit 30 \
  --workflow "Release"

gh run list \
  --repo dc-tec/openbao-operator \
  --limit 30 \
  --workflow "Release Please PR"

gh run list \
  --repo dc-tec/openbao-operator \
  --limit 30 \
  --workflow "Release Please Tag"

gh run list \
  --repo dc-tec/openbao-operator \
  --limit 30 \
  --workflow "Publish Edge"`}
>
  Start here when you need a quick view of the workflows that can mutate releases, tags, GHCR publication state, or public manifests.
</CommandBlock>

<CommandBlock
  language="bash"
  label="inspect"
  title="Inspect recent release state"
  code={`gh release list --repo dc-tec/openbao-operator --limit 20
git ls-remote --tags origin

crane ls ghcr.io/dc-tec/openbao-operator | tail -n 20
crane ls ghcr.io/dc-tec/openbao-init | tail -n 20
crane ls ghcr.io/dc-tec/openbao-backup | tail -n 20
crane ls ghcr.io/dc-tec/openbao-upgrade | tail -n 20`}
>
  Compare recent tags, draft releases, and registry publication state to what should have been produced by the workflows you trust.
</CommandBlock>

<DecisionTable
  title="What to verify before restoring automation"
  columns={["Area", "What to prove", "Where to check"]}
  rows={[
    {
      cells: [
        "Workflow integrity",
        "Pinned actions, workflow helper sources, and release logic still match reviewed repository state.",
        "Default branch commits, workflow diffs, and recent PR history.",
      ],
    },
    {
      cells: [
        "Release identities",
        "Only the tag app has semver-tag bypass and the PR app has no release-tag authority.",
        "Repository rulesets, app installation settings, and repository secrets.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Published subjects",
        "Recent images, charts, checksums, and releases still verify against the expected workflow identity and source ref.",
        "Release verification commands, provenance index, and GHCR digests.",
      ],
    },
    {
      cells: [
        "Credential hygiene",
        "Suspicious app keys, tokens, PATs, sessions, or SSH keys have been revoked or rotated.",
        "GitHub account sessions, SSH keys, app private keys, and repo/org secrets.",
      ],
    },
  ]}
/>

## Recovery checklist

- Re-enable GitHub Actions only after the triggering cause is understood and contained.
- Unsuspend the PR app first; keep the tag app suspended until release creation is safe again.
- Run a controlled prerelease or RC before restoring normal stable-release activity.
- Record the affected workflow run IDs, tags, releases, GHCR digests, and remediation steps in the incident notes.

<Callout type="warning" title="Single-maintainer constraint">

This runbook improves speed and consistency, but it does not create multi-party approval. If the maintainer account is compromised, treat repository settings, Actions secrets, GitHub Apps, and release state as potentially affected until proven otherwise.

</Callout>

<NextActions
  title="After containment"
  items={[
    {
      label: "Supply chain security",
      description: "Return to the underlying trust model once the immediate incident is contained.",
      to: "/contribute/supply-chain-security",
    },
    {
      label: "Release management",
      description: "Use the concrete release workflow when you are ready to run a controlled validation release.",
      to: "/contribute/release-management",
    },
    {
      label: "Continuous integration",
      description: "Inspect the implementation surface that may have allowed the incident path in the first place.",
      to: "/contribute/ci",
    },
  ]}
/>
