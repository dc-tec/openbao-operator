---
description: Validated cloud OpenBaoCluster architectures, starting with the manually validated Amazon EKS reference topologies.
---

# Cloud Architectures

Use these pages when you want a validated cloud topology with clear invariants, tradeoffs, and linked deployment recipes.

The current cloud scope comes from the project's manually validated Amazon EKS environment.

<div class="grid cards" markdown>

- :material-cloud-outline: **Amazon EKS Development**

    ---

    Shared terminating edge, AWS KMS auto-unseal, JWT bootstrap, and S3 backups for development and validation lanes.

    [:material-arrow-right: Open Architecture](amazon-eks-development-awskms-s3.md)

- :material-shield-check: **Amazon EKS Hardened**

    ---

    Dedicated passthrough edge, AWS KMS auto-unseal, OpenBao-managed ACME, signed helper images, and S3 backups.

    [:material-arrow-right: Open Architecture](amazon-eks-hardened-awskms-acme.md)

</div>
