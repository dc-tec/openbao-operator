---
title: Reference Architecture Catalog
hide_title: true
pageType: landing
description: Catalog of validated reference architectures, grouped by cloud and local environment.
---

<PageHero
  variant="landing"
  eyebrow="Validated Deployments / Architecture Catalog"
  title="Validated architecture catalog"
  lede="This catalog groups the reference architecture side of each validated deployment baseline. Use it when you want to review topology, invariants, and assumptions before the deployment recipe."
  actions={[
    {label: "Open cloud baselines", docId: "user-guide/validated-deployments/architectures/cloud/index", variant: "primary"},
    {label: "Open local baselines", docId: "user-guide/validated-deployments/architectures/local/index", variant: "secondary"},
  ]}
/>

<RouteList
  title="Architecture catalogs"
  items={[
    {
      eyebrow: "01",
      title: "Cloud baselines",
      description: "Validated EKS topologies with clear invariants and matching recipes.",
      docId: "user-guide/validated-deployments/architectures/cloud/index",
    },
    {
      eyebrow: "02",
      title: "Local baselines",
      description: "Validated k3d lanes for development, hardened rehearsal, and cross-cluster DR.",
      docId: "user-guide/validated-deployments/architectures/local/index",
    },
  ]}
/>
