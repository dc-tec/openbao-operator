---
title: Install the operator
description: Install OpenBao Operator 0.5.0 with the intended tenancy, platform, identity, CRDs, and admission policy contract.
eyebrow: Get started · Step 2
weight: 2
verifiedBy:
  - charts/openbao-operator/Chart.yaml
  - charts/openbao-operator/values.yaml
  - charts/openbao-operator/templates/controller/deployment.yaml
  - charts/openbao-operator/templates/provisioner/deployment.yaml
  - charts/openbao-operator/templates/admission
  - config/default/kustomization.yaml
  - config/overlays/custom-identity/kustomization.yaml
  - internal/adapter/config/builder.go
  - cmd/bao-backup/restore_flow.go
  - .github/workflows/release.yml
---

Install a pinned 0.5.0 release and verify the rendered namespace, identities, controllers, CRDs, and admission policies.
The core procedure uses Helm in the chart's default multi-tenant mode.

## Before you begin

- Confirm that the cluster meets the [compatibility requirements](../../reference/compatibility/). Kubernetes 1.33
  satisfies the chart constraint, while current release gates validate Kubernetes 1.34, 1.35, and 1.36.
- Use an identity that can create cluster-scoped CRDs, RBAC, and ValidatingAdmissionPolicies.
- Decide whether Helm, a release manifest, or a maintained Kustomize overlay owns future upgrades.
- Decide the tenancy model. Use the separate [single-tenant procedure](../single-tenant/) for one watched namespace.

## Choose an installation path

| Requirement | Path | What to verify |
| --- | --- | --- |
| Standard multi-tenant installation | Helm | Controller, Provisioner, CRDs, policies, and release identity |
| Default resources without Helm | Pinned `install.yaml` release asset | Published version and default namespace or identity |
| Custom namespace or prefix | `config/overlays/custom-identity` | Every subject and admission identity after rendering |
| Dedicated single namespace | Helm `tenancy.mode=single` or `config/overlays/single-tenant` | Controller-only runtime and `WATCH_NAMESPACE` |
| OpenShift | Helm with `platform=openshift`, or auto-detection | SCC-compatible workload security context |
| Local development | Source deployment | Development image and generated resources |

## Install with Helm

1. Set the release values.

   {{< command label="configure" title="Set the 0.5.0 release values" >}}
   export OPERATOR_RELEASE=openbao-operator
   export OPERATOR_NAMESPACE=openbao-operator-system
   export CHART_VERSION=0.5.0
   {{< /command >}}

2. Inspect the chart defaults when the platform needs overrides.

   {{< command label="inspect" title="Read the pinned values" >}}
   helm show values \
     oci://ghcr.io/dc-tec/charts/openbao-operator \
     --version "${CHART_VERSION}"
   {{< /command >}}

   Pin the chart and normally let its `appVersion` select the matching operator image. Set `image.tag` only for a
   controlled prerelease or test. The complete pinned reference is
   [`values.yaml`](https://github.com/dc-tec/openbao-operator/blob/0.5.0/charts/openbao-operator/values.yaml).

3. Render the installation before applying it when you use non-default values.

   {{< command label="inspect" title="Render the Helm release" >}}
   helm template "${OPERATOR_RELEASE}" \
     oci://ghcr.io/dc-tec/charts/openbao-operator \
     --version "${CHART_VERSION}" \
     --namespace "${OPERATOR_NAMESPACE}"
   {{< /command >}}

   Check the controller and Provisioner ServiceAccounts, RoleBinding subjects, admission-policy identity variables,
   projected token audience, images, and namespaces.

4. Install the chart.

   {{< command label="apply" title="Install OpenBao Operator" >}}
   helm upgrade --install "${OPERATOR_RELEASE}" \
     oci://ghcr.io/dc-tec/charts/openbao-operator \
     --version "${CHART_VERSION}" \
     --namespace "${OPERATOR_NAMESPACE}" \
     --create-namespace \
     --wait
   {{< /command >}}

5. Wait for both multi-tenant Deployments.

   {{< command label="verify" title="Verify the controller and Provisioner" >}}
   kubectl -n "${OPERATOR_NAMESPACE}" rollout status \
     deployment/openbao-operator-controller --timeout=2m
   kubectl -n "${OPERATOR_NAMESPACE}" rollout status \
     deployment/openbao-operator-provisioner --timeout=2m
   {{< /command >}}

6. Verify the installed APIs and admission policies.

   {{< command label="verify" title="Verify cluster-scoped resources" >}}
   kubectl get crd \
     openbaoclusters.openbao.org \
     openbaotenants.openbao.org \
     openbaorestores.openbao.org
   kubectl get validatingadmissionpolicies \
     -l app.kubernetes.io/instance="${OPERATOR_RELEASE}"
   {{< /command >}}

7. Verify the default controller JWT contract.

   {{< command label="inspect" title="Inspect the controller identity and audience" >}}
   kubectl -n "${OPERATOR_NAMESPACE}" get serviceaccount \
     openbao-operator-controller
   kubectl -n "${OPERATOR_NAMESPACE}" get deployment \
     openbao-operator-controller -o yaml
   {{< /command >}}

   Confirm that the Deployment uses the rendered ServiceAccount, mounts the projected `openbao-token`, and aligns its
   audience with `OPENBAO_JWT_AUDIENCE`. Continue with [operator authentication](../operator-authentication/) when you
   customize these values.

## Install the published manifest

Use the release asset when the platform wants the published default resources without a Helm release:

{{< command label="apply" title="Apply the 0.5.0 installer manifest" >}}
kubectl apply -f \
  https://github.com/dc-tec/openbao-operator/releases/download/0.5.0/install.yaml
{{< /command >}}

The manifest uses the repository's default operator namespace and identity. Do not rewrite the rendered YAML by hand
for a custom identity; maintain a Kustomize overlay instead.

## Install with a custom raw identity

Use `config/overlays/custom-identity` when the platform owns a different operator namespace or `namePrefix`.

{{< command label="inspect" title="Render the custom identity" >}}
kubectl kustomize config/overlays/custom-identity
{{< /command >}}

Before applying the overlay, confirm:

1. Controller and Provisioner ServiceAccounts have the intended names and namespace.
2. RoleBinding and ClusterRoleBinding subjects point at those ServiceAccounts.
3. Admission-policy variables use the same namespace and ServiceAccount names.
4. `OPENBAO_JWT_AUDIENCE` matches the projected `openbao-token` audience.
5. The OpenBao JWT role's bound subject matches the rendered controller identity.

Apply only after the render is internally consistent:

{{< command label="apply" title="Apply the custom identity overlay" >}}
kubectl apply -k config/overlays/custom-identity
{{< /command >}}

## Install on OpenShift

The chart defaults to `platform=auto`. Set `platform=openshift` when the installation must force OpenShift behavior:

{{< command label="apply" title="Force OpenShift rendering" >}}
helm upgrade --install openbao-operator \
  oci://ghcr.io/dc-tec/charts/openbao-operator \
  --version 0.5.0 \
  --namespace openbao-operator-system \
  --create-namespace \
  --set platform=openshift
{{< /command >}}

OpenShift mode omits fixed `runAsUser` and `fsGroup` IDs so the Security Context Constraint can assign namespace-scoped
IDs. Validate the result against the target cluster's SCC and admission configuration.

## Install from source for development

Use this path only for local development and contribution:

{{< command label="apply" title="Deploy a development image" >}}
make install
make deploy IMG=ghcr.io/dc-tec/openbao-operator:dev
{{< /command >}}

## Upgrade the operator

Helm does not upgrade installed CRDs. For every release with CRD changes, apply the release CRDs before the controller:

{{< callout type="warning" title="Update 0.4.2 restore policies before the controller" >}}
OpenBao Operator 0.4.2 generated restore policies with only `update` on
`sys/storage/raft/snapshot-force`. In 0.5.0, a restore with `force` omitted or set to `false` uses
`sys/storage/raft/snapshot`. Add `update` on the normal endpoint through an authenticated administration path before
you upgrade the controller. Self-init does not update an existing policy.
{{< /callout >}}

{{< command label="upgrade" title="Upgrade to 0.5.0" >}}
kubectl apply -f \
  https://github.com/dc-tec/openbao-operator/releases/download/0.5.0/crds.yaml
helm upgrade openbao-operator \
  oci://ghcr.io/dc-tec/charts/openbao-operator \
  --version 0.5.0 \
  --namespace openbao-operator-system \
  --reuse-values \
  --wait
{{< /command >}}

Re-render custom identities and review OpenBao-side policy changes before upgrading. Self-init does not update existing
OpenBao policies.

## Uninstall the operator

{{< command label="remove" title="Remove the Helm release" >}}
helm uninstall openbao-operator --namespace openbao-operator-system
{{< /command >}}

{{< callout type="warning" title="Helm retains all three CRDs" >}}
Deleting the CRDs also deletes their custom resources from the Kubernetes API and can trigger or bypass lifecycle
expectations depending on what remains. Inventory every `OpenBaoCluster`, `OpenBaoTenant`, and `OpenBaoRestore` before a
separate CRD-deletion operation.
{{< /callout >}}

## Troubleshoot installation

| Symptom | Check |
| --- | --- |
| Controller starts but Provisioner is absent | Confirm that the chart did not render `tenancy.mode=single` |
| Pods run but admission rejects ordinary resources | Inspect policy bindings, rendered identity variables, and the API server's ValidatingAdmissionPolicy support |
| Custom names break reconciliation | Compare every ServiceAccount, RoleBinding subject, admission variable, and JWT bound subject |
| OpenShift rejects Pod identity fields | Confirm auto-detection or set `platform=openshift`, then review SCC ownership |
| Helm upgrade leaves an old API schema | Apply the release `crds.yaml` before retrying the controller upgrade |

In multi-tenant mode, continue with [namespace onboarding](../onboard-namespace/).
