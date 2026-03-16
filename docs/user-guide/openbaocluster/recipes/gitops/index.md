---
description: Step-by-step GitOps deployment recipes for OpenBaoCluster, including ArgoCD-managed validation environments.
---

# GitOps Recipes

Use this category for deployment flows where GitOps is the supported operating model, not just an optional delivery mechanism.

The current GitOps recipe scope starts with the ArgoCD-managed local validation lane from `openbao-operator-test`.

These pages complement the local and cloud architectures. They document how those environments are reconciled through Git rather than introducing a separate OpenBao runtime topology.

<div class="grid cards" markdown>

- :material-source-branch: **ArgoCD on k3d**

    ---

    Bootstrap the local k3d cluster, install ArgoCD, and reconcile the OpenBao validation lanes through an `ApplicationSet`.

    [:material-arrow-right: Open Recipe](argocd-k3d-bootstrap.md)

</div>
