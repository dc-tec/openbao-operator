---
description: Step-by-step OpenBaoCluster recipes grouped by environment, operating model, and day-2 workflow type.
---

# Recipes

These recipes are grounded in the project's manual validation environment. Use them when you want a complete, step-by-step flow instead of a feature reference page.

If you want the tested topology, invariants, and validation scope first, start with [Validated Architectures](../architectures/index.md).

<div class="grid cards" markdown>

- :material-cloud-outline: **Cloud**

    ---

    Cloud deployment flows, starting with the validated Amazon EKS development and hardened lanes.

    [:material-arrow-right: Open Category](cloud/index.md)

- :material-laptop: **Local**

    ---

    Local development and validation flows, including self-init, Hardened, and passthrough examples.

    [:material-arrow-right: Open Category](local/index.md)

- :material-source-branch: **GitOps**

    ---

    GitOps-oriented flows where ArgoCD or a similar control plane is the operating model.

    [:material-arrow-right: Open Category](gitops/index.md)

- :material-wrench: **Operations**

    ---

    Day-2 workflows such as scheduled backups and snapshot-driven restore procedures.

    [:material-arrow-right: Open Category](operations/index.md)

</div>
