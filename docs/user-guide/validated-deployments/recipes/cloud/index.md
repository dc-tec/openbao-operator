---
description: Step-by-step cloud deployment recipes for OpenBaoCluster, starting with the validated Amazon EKS flows.
---

# Cloud Recipes

Use these recipes when you want a procedural deployment flow for a cloud environment.

Start with [Cloud Architectures](../../architectures/cloud/index.md) if you want the tested topology and invariants first.

<div class="grid cards" markdown>

- :material-cloud-outline: **Amazon EKS Development**

    ---

    Deploy a Development-profile cluster on Amazon EKS with AWS KMS auto-unseal, Gateway API exposure, JWT bootstrap, and S3 backups.

    [:material-arrow-right: Open Recipe](amazon-eks-development-awskms-s3.md)

- :material-shield-check: **Amazon EKS Hardened**

    ---

    Deploy a Hardened cluster on Amazon EKS with AWS KMS auto-unseal, a dedicated passthrough Gateway, OpenBao-managed ACME, and S3 backups.

    [:material-arrow-right: Open Recipe](amazon-eks-hardened-awskms-acme.md)

</div>
