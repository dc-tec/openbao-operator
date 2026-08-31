---
title: Install the operator
description: Evaluate unreleased main through the edge channel or deploy a development image from source.
eyebrow: Get started · Step 2
weight: 2
verifiedBy:
  - .github/workflows/ci.yml
  - .github/workflows/publish-edge.yml
  - hack/ci/generate-channel-manifests.sh
  - charts/openbao-operator/values.yaml
  - config/default/kustomization.yaml
---

Next tracks unreleased behavior on `main`. Use the edge channel for an executable evaluation install, or build from
source when you are developing the operator. Do not treat either path as a stable production contract.

{{< callout type="warning" title="Use stable documentation for production" >}}
The edge channel is mutable and advances after successful `main` validation. Use the current stable documentation
and a pinned release for production. OpenBao Operator 0.5.0 is the current stable release.
{{< /callout >}}

## Before you begin

- Confirm that the cluster meets the [Next compatibility requirements](../../reference/compatibility/).
- Use an identity that can create cluster-scoped CRDs, RBAC, and ValidatingAdmissionPolicies.
- Install `kubectl`. Source deployments also require the repository toolchain and a registry the cluster can pull
  from.
- Decide the tenancy model. Use the [single-tenant procedure](../single-tenant/) for one watched namespace.

## Install the latest validated edge build

The edge publisher promotes images and generates manifests from the same successful `main` commit. Inspect the channel
metadata before applying it so you know the exact commit and image digests under evaluation.

1. Set the edge channel URL and inspect its metadata.

   {{< command label="inspect" title="Review the current edge build" >}}
   export EDGE_ROOT=https://dc-tec.github.io/openbao-operator/edge/latest
   curl --fail --silent --show-error "${EDGE_ROOT}/metadata.json"
   {{< /command >}}

   Record the `sha`, generated time, and image digests. Follow the
   [supply-chain verification](../../security/supply-chain/) procedure when the evaluation requires provenance
   verification.

2. Apply the generated multi-tenant installer.

   {{< command label="apply" title="Install the edge manifest" >}}
   kubectl apply -f "${EDGE_ROOT}/install.yaml"
   {{< /command >}}

3. Wait for both multi-tenant controllers.

   {{< command label="verify" title="Verify the controller and Provisioner" >}}
   kubectl -n openbao-operator-system rollout status \
     deployment/openbao-operator-controller --timeout=2m
   kubectl -n openbao-operator-system rollout status \
     deployment/openbao-operator-provisioner --timeout=2m
   {{< /command >}}

4. Verify the installed APIs and admission policies.

   {{< command label="verify" title="Verify cluster-scoped resources" >}}
   kubectl get crd \
     openbaoclusters.openbao.org \
     openbaotenants.openbao.org \
     openbaorestores.openbao.org
   kubectl get validatingadmissionpolicies
   {{< /command >}}

5. Verify the controller identity.

   {{< command label="inspect" title="Inspect the controller identity" >}}
   kubectl -n openbao-operator-system get serviceaccount \
     openbao-operator-controller
   kubectl -n openbao-operator-system get deployment \
     openbao-operator-controller -o yaml
   {{< /command >}}

   Confirm that the Deployment uses the rendered ServiceAccount and projected `openbao-token`. Continue with
   [operator authentication](../operator-authentication/) when you customize the JWT audience or identity.

## Deploy from source

Use a source deployment when you need a local change or an exact checkout that has not reached the edge channel.

1. Check out the intended commit and prepare the toolchain.

   {{< command label="configure" title="Prepare the source checkout" >}}
   git clone https://github.com/dc-tec/openbao-operator.git
   cd openbao-operator
   git checkout <commit>
   make bootstrap
   {{< /command >}}

2. Build and push an image that the cluster can pull.

   {{< command label="build" title="Publish the development image" >}}
   export IMG=<registry>/openbao-operator:<commit>
   make docker-build docker-push IMG="${IMG}"
   {{< /command >}}

   For a local Kind cluster, load the image into every node instead of pushing it, then use the same image reference
   for deployment.

3. Deploy the generated resources and the selected image.

   {{< command label="apply" title="Deploy the source build" >}}
   make deploy IMG="${IMG}" OPERATOR_VERSION=edge
   {{< /command >}}

## Render the local Helm contract

Use the checked-out chart when you need to evaluate Helm rendering, including the single-tenant or OpenShift paths.
The edge image and operator version keep helper-image selection aligned with the unreleased build.

{{< command label="inspect" title="Render the local edge chart" >}}
helm template openbao-operator charts/openbao-operator \
  --namespace openbao-operator-system \
  --include-crds \
  --set image.tag=edge \
  --set operatorVersion=edge
{{< /command >}}

Review the controller and Provisioner ServiceAccounts, RoleBinding subjects, admission-policy identities, projected
token audience, images, and namespaces before applying the render.

## Refresh an edge installation

The edge channel is mutable. Re-read `metadata.json`, then apply CRDs before the complete installer when the recorded
commit changes.

{{< command label="upgrade" title="Refresh to the current edge build" >}}
kubectl apply -f "${EDGE_ROOT}/crds.yaml"
kubectl apply -f "${EDGE_ROOT}/install.yaml"
{{< /command >}}

Re-render custom identities and review OpenBao-side policy changes before refreshing. Self-init does not update
existing OpenBao policies.

## Remove the source or edge deployment

Use the same generated manifest that installed the operator. Inventory every `OpenBaoCluster`, `OpenBaoTenant`, and
`OpenBaoRestore` before deleting CRDs because CR deletion can trigger lifecycle behavior.

{{< command label="remove" title="Remove the edge deployment" >}}
kubectl delete -f "${EDGE_ROOT}/install.yaml"
{{< /command >}}

## Troubleshoot installation

| Symptom | Check |
| --- | --- |
| Controller starts but Provisioner is absent | Confirm that you did not render `tenancy.mode=single` |
| Pods cannot pull the source image | Push it to a cluster-reachable registry or load it into every local node |
| Pods run but admission rejects ordinary resources | Inspect policy bindings, rendered identity variables, and API-server ValidatingAdmissionPolicy support |
| Custom names break reconciliation | Compare every ServiceAccount, RoleBinding subject, admission variable, and JWT bound subject |
| OpenShift rejects Pod identity fields | Render `platform=openshift`, then review SCC ownership |

In multi-tenant mode, continue with [namespace onboarding](../onboard-namespace/).
