---
title: Cloud Baselines
hide_title: true
pageType: landing
description: Validated cloud baselines for OpenBao Operator, pairing each tested topology with the recipe that reproduces it.
---

<PageHero
  variant="landing"
  eyebrow="Validated Deployments / Cloud Baselines"
  title="Validated cloud baselines"
  lede="Amazon EKS validated baselines are listed here, each linked to its matching deployment recipe."
  actions={[
    {label: "Open EKS Development", docId: "user-guide/validated-deployments/architectures/cloud/amazon-eks-development-awskms-s3", variant: "primary"},
    {label: "Open EKS Hardened", docId: "user-guide/validated-deployments/architectures/cloud/amazon-eks-hardened-awskms-acme", variant: "secondary"},
  ]}
>
  <Checklist
    title="Use cloud baselines to"
    items={[
      "compare validated development and hardened EKS topologies",
      "see the exact trust boundary, edge model, and storage assumptions before deployment",
      "move from architecture to deployment recipe without losing the lane context",
    ]}
  />
</PageHero>

<RouteList
  title="Validated cloud lanes"
  items={[
    {
      eyebrow: "01",
      title: "EKS Development",
      description: "EKS development baseline with shared terminating edge, AWS KMS auto-unseal, JWT bootstrap, and S3 backups.",
      docId: "user-guide/validated-deployments/architectures/cloud/amazon-eks-development-awskms-s3",
      actionLabel: "Open lane",
    },
    {
      eyebrow: "02",
      title: "EKS Hardened",
      description: "EKS hardened baseline with dedicated passthrough edge, AWS KMS auto-unseal, OpenBao-managed ACME, and S3 backups.",
      docId: "user-guide/validated-deployments/architectures/cloud/amazon-eks-hardened-awskms-acme",
      actionLabel: "Open lane",
    },
  ]}
/>

<NextActions
  title="Related catalogs"
  items={[
    {
      label: "Cloud recipe catalog",
      description: "Browse the cloud deployment procedures directly if you already know which lane you want.",
      docId: "user-guide/validated-deployments/recipes/cloud/index",
    },
    {
      label: "Validated deployments overview",
      description: "Return to the section entry if you still need to compare cloud and local baselines.",
      docId: "user-guide/validated-deployments/index",
    },
  ]}
/>
