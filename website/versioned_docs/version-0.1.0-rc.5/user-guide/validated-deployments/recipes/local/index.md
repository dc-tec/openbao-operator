---
title: Local Recipe Catalog
hide_title: true
pageType: landing
description: Local deployment recipes for the validated k3d lanes.
---

<PageHero
  eyebrow="Validated Deployments / Local Recipe Catalog"
  title="Run the local procedure that matches the validated lane you want to rehearse."
  lede="These recipes reproduce the local k3d validation lanes. They are useful because they follow the exact assumptions exercised by the project environment, but they should still be read as lane procedures, not as generic operator setup."
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
