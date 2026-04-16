---
title: Validated Procedure Catalog
hide_title: true
pageType: landing
description: Catalog of validated procedures that still need lane-specific treatment, with generic backup and restore workflows handed off to the main operator docs.
---

<PageHero
  variant="landing"
  eyebrow="Validated Deployments / Procedure Catalog"
  title="Validated runbooks"
  lede="This catalog contains the procedures that still depend on a specific validated baseline. Generic backup and restore guidance lives in the main `Operate` and `Recovery & Restore` sections."
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
  title="Main operator docs for generic procedures"
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
