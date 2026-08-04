---
title: Deployment Recipe Catalog
hide_title: true
pageType: landing
description: Catalog of validated deployment recipes, grouped by cloud and local environment.
---

<PageHero
  variant="landing"
  eyebrow="Validated Deployments / Recipe Catalog"
  title="Validated recipe catalog"
  lede="Deployment recipes are grouped by validated baseline. Keep the matching reference architecture nearby while you deploy."
  actions={[
    {label: "Open cloud recipe catalog", docId: "user-guide/validated-deployments/recipes/cloud/index", variant: "primary"},
    {label: "Open local recipe catalog", docId: "user-guide/validated-deployments/recipes/local/index", variant: "secondary"},
  ]}
/>

<RouteList
  title="Recipe catalogs"
  items={[
    {
      eyebrow: "01",
      title: "Cloud recipes",
      description: "Deployment procedures for the validated EKS development and hardened lanes.",
      docId: "user-guide/validated-deployments/recipes/cloud/index",
    },
    {
      eyebrow: "02",
      title: "Local recipes",
      description: "Deployment procedures for the validated k3d development, hardened, and cross-cluster DR baselines.",
      docId: "user-guide/validated-deployments/recipes/local/index",
    },
  ]}
/>
