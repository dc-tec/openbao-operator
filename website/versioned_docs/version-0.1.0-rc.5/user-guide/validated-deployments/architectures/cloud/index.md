---
title: Cloud Baselines
hide_title: true
pageType: landing
description: Validated cloud baselines for OpenBao Operator, pairing each tested topology with the recipe that reproduces it.
---

<PageHero
  variant="landing"
  eyebrow="Validated Deployments / Cloud Baselines"
  title="Pick the cloud lane that matches the posture you want to prove."
  lede="The current cloud validated scope comes from Amazon EKS. Each lane keeps the tested topology and the matching deployment recipe adjacent, so you can confirm the architecture first and then reproduce it without jumping across unrelated nav branches."
  actions={[
    {label: "Open EKS Development", docId: "user-guide/validated-deployments/architectures/cloud/amazon-eks-development-awskms-s3", variant: "primary"},
    {label: "Open EKS Hardened", docId: "user-guide/validated-deployments/architectures/cloud/amazon-eks-hardened-awskms-acme", variant: "secondary"},
  ]}
>
  <Checklist
    title="Use cloud baselines when you need to"
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
      description: "Shared terminating edge, AWS KMS auto-unseal, JWT bootstrap, and S3 backups for a realistic but non-production lane.",
      docId: "user-guide/validated-deployments/architectures/cloud/amazon-eks-development-awskms-s3",
      actionLabel: "Open lane",
    },
    {
      eyebrow: "02",
      title: "EKS Hardened",
      description: "Dedicated passthrough edge, AWS KMS auto-unseal, OpenBao-managed ACME, and the tighter production posture used in the hardened cloud lane.",
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
