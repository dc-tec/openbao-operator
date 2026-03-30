---
title: Validated Procedure Catalog
hide_title: true
pageType: landing
description: Minimal catalog for validated procedures that still need lane-specific treatment, with generic backup and restore workflows handed off to the main operator docs.
---

<PageHero
  variant="landing"
  eyebrow="Validated Deployments / Procedure Catalog"
  title="Use this catalog only for procedures that still depend on a validated lane."
  lede="Generic backup and restore workflows now live in the main `Operate` and `Recovery & Restore` sections. This catalog remains only for the k3d cross-cluster DR restore runbook, because that workflow still depends on the assumptions of one validated lane."
  actions={[
    {label: "Open DR restore runbook", docId: "user-guide/validated-deployments/runbooks/cross-cluster-dr-restore-rustfs", variant: "primary"},
    {label: "Open generic restore guide", docId: "user-guide/openbaorestore/restore", variant: "secondary"},
  ]}
/>

<RouteList
  title="Lane-specific procedure"
  items={[
    {
      eyebrow: "01",
      title: "Cross-cluster DR restore",
      description: "Lane-specific restore procedure for the validated k3d DR environment with shared Transit and shared snapshot storage.",
      docId: "user-guide/validated-deployments/runbooks/cross-cluster-dr-restore-rustfs",
    },
  ]}
/>

<NextActions
  title="Use the main operator docs for generic procedures"
  items={[
    {
      label: "Backup operations",
      description: "Canonical backup scheduling, credential, retention, and verification guidance.",
      docId: "user-guide/openbaocluster/operations/backups",
    },
    {
      label: "Restore from backup",
      description: "Canonical restore workflow for `OpenBaoRestore`, storage sources, and destructive restore behavior.",
      docId: "user-guide/openbaorestore/restore",
    },
  ]}
/>
