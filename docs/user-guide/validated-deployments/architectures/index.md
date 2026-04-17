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
  lede="Reference architectures are grouped by validated deployment baseline. Review topology, invariants, and assumptions here before following a recipe."
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
      description: "Amazon EKS topologies with matching recipes.",
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
