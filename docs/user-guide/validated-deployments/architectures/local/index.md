---
title: Local Baselines
hide_title: true
pageType: landing
description: Validated local baselines for k3d, including development, hardened rehearsal, and cross-cluster DR lanes.
---

<PageHero
  variant="landing"
  eyebrow="Validated Deployments / Local Baselines"
  title="Use local lanes for rehearsal, validation, and DR proof, not as accidental production defaults."
  lede="The local validated scope comes from the project's k3d environment. These lanes are valuable because they prove concrete behaviors such as hardened bootstrap, passthrough access, ACME issuance, and cross-cluster restore. They are not a substitute for making an explicit production platform choice."
  actions={[
    {label: "Open k3d Development", docId: "user-guide/validated-deployments/architectures/local/k3d-development-shared-edge-rustfs", variant: "primary"},
    {label: "Open k3d Cross-Cluster DR", docId: "user-guide/validated-deployments/architectures/local/k3d-cross-cluster-dr-transit-rustfs", variant: "secondary"},
  ]}
>
  <Checklist
    title="Use local baselines when you need to"
    items={[
      "rehearse a development or hardened lane on workstation-grade infrastructure",
      "prove a boundary such as external TLS passthrough, internal ACME, or shared Transit unseal",
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
      description: "Shared terminating edge, RustFS backups, JWT bootstrap, and a development-profile lane for rehearsal and integration work.",
      docId: "user-guide/validated-deployments/architectures/local/k3d-development-shared-edge-rustfs",
      actionLabel: "Open lane",
    },
    {
      eyebrow: "02",
      title: "k3d Hardened / External TLS",
      description: "Transit auto-unseal, external TLS Secrets, and user-managed passthrough for the closest local analogue to a hardened external-certificate deployment.",
      docId: "user-guide/validated-deployments/architectures/local/k3d-hardened-transit-external-tls",
      actionLabel: "Open lane",
    },
    {
      eyebrow: "03",
      title: "k3d Hardened / ACME",
      description: "Transit auto-unseal with OpenBao-managed ACME and validated hostname resolution in the local hardened ACME lane.",
      docId: "user-guide/validated-deployments/architectures/local/k3d-hardened-transit-acme",
      actionLabel: "Open lane",
    },
    {
      eyebrow: "04",
      title: "k3d Cross-Cluster DR",
      description: "A proving lane for shared Transit, shared snapshot storage, and restore rehearsal across separate source and target clusters.",
      docId: "user-guide/validated-deployments/architectures/local/k3d-cross-cluster-dr-transit-rustfs",
      actionLabel: "Open lane",
    },
  ]}
/>

<Callout type="note" title="Only the DR restore runbook stays lane-specific here">

Generic backup and restore procedures belong in the main `Operate` docs. The cross-cluster DR restore procedure remains in this section because it depends on the exact validated DR lane assumptions.

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
