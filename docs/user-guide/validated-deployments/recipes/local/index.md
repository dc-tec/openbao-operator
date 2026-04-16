---
title: Local Recipe Catalog
hide_title: true
pageType: landing
description: Local deployment recipes for the validated k3d lanes.
---

<PageHero
  variant="landing"
  eyebrow="Validated Deployments / Local Recipe Catalog"
  title="Validated local recipes"
  lede="This catalog contains the deployment recipes for the validated k3d baselines. These procedures reproduce the project-tested local environments and should be read together with the matching baseline."
  actions={[
    {label: "Open k3d Development recipe", docId: "user-guide/validated-deployments/recipes/local/development-self-init-userpass", variant: "primary"},
    {label: "Open k3d Cross-Cluster DR bootstrap", docId: "user-guide/validated-deployments/recipes/local/k3d-cross-cluster-dr-bootstrap", variant: "secondary"},
  ]}
/>

<RouteList
  title="Local recipes"
  items={[
    {
      eyebrow: "01",
      title: "k3d Development recipe",
      description: "Bootstrap the development-profile local lane with self-init, userpass, shared edge, and RustFS backups.",
      docId: "user-guide/validated-deployments/recipes/local/development-self-init-userpass",
    },
    {
      eyebrow: "02",
      title: "k3d Hardened / External TLS recipe",
      description: "Deploy the hardened local lane with Transit auto-unseal, self-init, and externally managed TLS Secrets.",
      docId: "user-guide/validated-deployments/recipes/local/hardened-transit-external-tls",
    },
    {
      eyebrow: "03",
      title: "k3d Hardened / ACME recipe",
      description: "Deploy the hardened local lane that relies on OpenBao-managed ACME and validated hostname resolution.",
      docId: "user-guide/validated-deployments/recipes/local/hardened-transit-acme-tls",
    },
    {
      eyebrow: "04",
      title: "k3d Cross-Cluster DR bootstrap",
      description: "Bootstrap the source and target clusters for the validated DR proving lane.",
      docId: "user-guide/validated-deployments/recipes/local/k3d-cross-cluster-dr-bootstrap",
    },
  ]}
/>
