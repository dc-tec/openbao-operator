---
title: Lifecycle Architecture
hide_title: true
pageType: landing
journey: architecture
description: Lifecycle overview for OpenBao Operator from Day 0 provisioning through Day 1 creation, Day 2 operations, and backup-and-restore flows.
---

<PageHero
  variant="landing"
  eyebrow="Architecture / Lifecycle"
  title="Follow the operator from tenant provisioning to day 2 operations and backup-driven restore."
  lede="These pages explain the lifecycle from the controller and manager perspective. Use them to understand which internal responsibilities own Day 0 provisioning, Day 1 creation, Day 2 operations, and backup-and-restore flows rather than how an operator user performs those tasks."
  actions={[
    {label: 'Start with Day 0 provisioning', docId: 'architecture/lifecycle/day0-provisioning', variant: 'primary'},
    {label: 'Open Day 2 operations', docId: 'architecture/lifecycle/day2-operations', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Lifecycle pages explain"
    items={[
      'which controller and manager own each phase',
      'where security and tenant boundaries enter the lifecycle',
      'how upgrades, backups, and restore fit into day 2 behavior',
      'how the internal lifecycle maps back to the user-facing docs',
    ]}
  />
</PageHero>

<RouteList
  title="Lifecycle flows"
  items={[
    {
      eyebrow: '00',
      title: 'Day 0 provisioning',
      description: 'Tenant onboarding, namespace creation, RBAC delegation, and the provisioner boundary before a cluster exists.',
      docId: 'architecture/lifecycle/day0-provisioning',
    },
    {
      eyebrow: '01',
      title: 'Day 1 creation',
      description: 'Cluster initialization, PKI bootstrapping, and the early readiness path for a new OpenBaoCluster.',
      docId: 'architecture/lifecycle/day1-creation',
    },
    {
      eyebrow: '02',
      title: 'Day 2 operations',
      description: 'Upgrade strategies, maintenance mode, and the internal flow behind normal production operations.',
      docId: 'architecture/lifecycle/day2-operations',
    },
    {
      eyebrow: 'N',
      title: 'Backups and restore',
      description: 'Snapshot scheduling, restore dependencies, and the backup-driven durability path after the cluster is live.',
      docId: 'architecture/lifecycle/dayN-backups',
    },
  ]}
/>

<NextActions
  title="Related architecture pages"
  items={[
    {
      label: 'Component design',
      description: 'Start with the control-plane split when you need the static system boundaries behind these lifecycle flows.',
      docId: 'architecture/components',
    },
    {
      label: 'Operation lifecycle coordination',
      description: 'See how upgrade, backup, and restore managers share the same lock and retry model.',
      docId: 'architecture/operation-lifecycle',
    },
    {
      label: 'Open Operate',
      description: 'Compare the internal lifecycle model with the user-facing operating guides for upgrades, backups, and restore.',
      docId: 'user-guide/openbaocluster/operations/index',
    },
  ]}
/>
