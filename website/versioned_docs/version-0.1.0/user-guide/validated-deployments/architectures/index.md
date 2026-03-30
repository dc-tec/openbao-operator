---
title: Reference Architecture Catalog
hide_title: true
pageType: landing
description: Catalog of validated reference architectures, grouped by cloud and local environment.
---

<PageHero
  variant="landing"
  eyebrow="Validated Deployments / Architecture Catalog"
  title="Use the architecture catalog when you want topology first."
  lede="This page exists for readers who arrive looking specifically for the reference architecture side of a validated lane. The main section navigation is now lane-first, so you can move from topology to recipe without switching sections in your head."
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
      description: "Validated k3d lanes for development, hardened rehearsal, and cross-cluster DR proof.",
      docId: "user-guide/validated-deployments/architectures/local/index",
    },
  ]}
/>
