---
title: Cloud Recipe Catalog
hide_title: true
pageType: landing
description: Cloud deployment recipes for the validated Amazon EKS lanes.
---

<PageHero
  variant="landing"
  eyebrow="Validated Deployments / Cloud Recipe Catalog"
  title="Validated cloud recipes"
  lede="Deployment recipes for the validated Amazon EKS baselines are listed here. Keep the matching reference architecture nearby during deployment."
  actions={[
    {label: "Open EKS Development recipe", docId: "user-guide/validated-deployments/recipes/cloud/amazon-eks-development-awskms-s3", variant: "primary"},
    {label: "Open EKS Hardened recipe", docId: "user-guide/validated-deployments/recipes/cloud/amazon-eks-hardened-awskms-acme", variant: "secondary"},
  ]}
/>

<RouteList
  title="Cloud recipes"
  items={[
    {
      eyebrow: "01",
      title: "EKS Development recipe",
      description: "Step through the development-profile EKS lane with AWS KMS, Gateway exposure, and S3 backups.",
      docId: "user-guide/validated-deployments/recipes/cloud/amazon-eks-development-awskms-s3",
    },
    {
      eyebrow: "02",
      title: "EKS Hardened recipe",
      description: "Deploy the hardened EKS lane with passthrough edge, AWS KMS, ACME, and the hardened production posture.",
      docId: "user-guide/validated-deployments/recipes/cloud/amazon-eks-hardened-awskms-acme",
    },
  ]}
/>
