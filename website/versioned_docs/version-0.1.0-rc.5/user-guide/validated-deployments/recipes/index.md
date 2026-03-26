---
title: Deployment Recipe Catalog
hide_title: true
pageType: landing
description: Catalog of validated deployment recipes, grouped by cloud and local environment.
---

<PageHero
  eyebrow="Validated Deployments / Recipe Catalog"
  title="Use the recipe catalog when you already know the lane and just need the procedure."
  lede="Recipes are the task-oriented half of a validated lane. They assume you either already know the topology or will keep the matching reference architecture close while you apply the manifests."
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
      description: "Deployment procedures for the validated k3d development, hardened, and cross-cluster DR lanes.",
      docId: "user-guide/validated-deployments/recipes/local/index",
    },
  ]}
/>
