---
title: Local Baselines
hide_title: true
pageType: landing
description: Validated local baselines for k3d, including development, hardened rehearsal, and cross-cluster DR lanes.
---

<PageHero
  variant="landing"
  eyebrow="Validated Deployments / Local Baselines"
  title="Validated local baselines"
  lede="Validated k3d baselines are listed here for development, hardened rehearsal, and cross-cluster DR."
  actions={[
    {label: "Open k3d Development", docId: "user-guide/validated-deployments/architectures/local/k3d-development-shared-edge-rustfs", variant: "primary"},
    {label: "Open k3d Cross-Cluster DR", docId: "user-guide/validated-deployments/architectures/local/k3d-cross-cluster-dr-transit-rustfs", variant: "secondary"},
  ]}
>
  <Checklist
    title="Use local baselines to"
    items={[
      "rehearse a development or hardened lane on workstation-grade infrastructure",
      "exercise a boundary such as external TLS passthrough, internal ACME, or shared Transit unseal",
      "practice restore and cutover behavior in the validated cross-cluster DR lane",
    ]}
  />
</PageHero>

<RouteList
  title="Validated local lanes"
  items={[
    {
      eyebrow: "01",
      title: "k3d Development",
      description: "Development profile with shared terminating edge, RustFS backups, and JWT bootstrap for rehearsal and integration work.",
      docId: "user-guide/validated-deployments/architectures/local/k3d-development-shared-edge-rustfs",
      actionLabel: "Open lane",
    },
    {
      eyebrow: "02",
      title: "k3d Hardened / External TLS",
      description: "Hardened external-certificate baseline with Transit auto-unseal, external TLS Secrets, and user-managed passthrough.",
      docId: "user-guide/validated-deployments/architectures/local/k3d-hardened-transit-external-tls",
      actionLabel: "Open lane",
    },
    {
      eyebrow: "03",
      title: "k3d Hardened / ACME",
      description: "Hardened ACME baseline with Transit auto-unseal, OpenBao-managed ACME, and local hostname resolution.",
      docId: "user-guide/validated-deployments/architectures/local/k3d-hardened-transit-acme",
      actionLabel: "Open lane",
    },
    {
      eyebrow: "04",
      title: "k3d Cross-Cluster DR",
      description: "Cross-cluster DR baseline with shared Transit, shared snapshot storage, and restore rehearsal across source and target clusters.",
      docId: "user-guide/validated-deployments/architectures/local/k3d-cross-cluster-dr-transit-rustfs",
      actionLabel: "Open lane",
    },
  ]}
/>

<Callout type="note" title="Lane-specific runbook scope">

Generic backup and restore procedures belong in the main `Operate` docs. The cross-cluster DR restore procedure stays here because it depends on the DR-lane assumptions used in this catalog.

</Callout>

<NextActions
  title="Related catalogs"
  items={[
    {
      label: "Local recipe catalog",
      description: "Browse the local deployment procedures directly if you already know which lane you want to reproduce.",
      docId: "user-guide/validated-deployments/recipes/local/index",
    },
    {
      label: "Generic restore guide",
      description: "Use the main restore docs for the canonical workflow outside the specific DR proving lane.",
      docId: "user-guide/openbaorestore/restore",
    },
  ]}
/>
