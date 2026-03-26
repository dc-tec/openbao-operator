---
title: Air-Gapped and Private Registries
hide_title: true
pageType: task
journey: configure
description: Mirror operator and workload images, set the right repository defaults, and wire pull secrets before you move clusters into disconnected or private-registry environments.
---

<PageHeader
  title="Mirror every image surface before you call the environment disconnected-ready."
  lede="An air-gapped or private-registry deployment is not just one image override. The operator image, the default OpenBao workload image, and the helper executors for init, backup, and upgrade each have their own source of truth. Use this page to make those defaults explicit before you need to promote clusters through a disconnected path."
/>

<DecisionTable
  title="Plan every image surface explicitly"
  columns={["Surface", "Defaults from", "Override it here", "Watch for"]}
  rows={[
    {
      cells: [
        "Operator controller and provisioner images",
        "The Helm chart image values used during installation.",
        "Set `image.repository`, `image.tag`, and install-level `imagePullSecrets` on the chart.",
        "In multi-tenant mode, both controller and provisioner deployments must be able to pull from the mirrored registry.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Default OpenBao workload image",
        "The cluster version plus the `RELATED_IMAGE_OPENBAO` repository default.",
        "Set the repository default on the operator install or set `spec.image` per cluster.",
        "If `spec.image` is omitted, the operator still derives the final image from `spec.version` and the mirrored repository default.",
      ],
    },
    {
      cells: [
        "Helper executor images",
        "The `OPERATOR_INIT_IMAGE_REPOSITORY`, `OPERATOR_BACKUP_IMAGE_REPOSITORY`, and `OPERATOR_UPGRADE_IMAGE_REPOSITORY` defaults.",
        "Set install-wide defaults or override `spec.initContainer.image`, `spec.backup.image`, and `spec.upgrade.image` per cluster.",
        "Restore jobs use their own image surface in the restore workflow and should be reviewed there before a DR event.",
      ],
    },
    {
      cells: [
        "Registry authentication",
        "The operator install uses chart-level `imagePullSecrets`; each cluster uses `spec.imagePullSecrets`.",
        "Create Docker registry Secrets in the namespace that will pull the images.",
        "Do not assume the operator namespace and tenant namespaces can share pull secrets implicitly.",
      ],
      emphasis: "caution",
    },
  ]}
/>

## Set install-wide defaults

<CommandBlock
  language="yaml"
  label="configure"
  title="Install the operator with mirrored image defaults"
  code={`image:
  repository: my-registry.corp/openbao-operator
  tag: "0.1.0-rc.5"
imagePullSecrets:
  - name: operator-registry-creds

controller:
  extraEnv:
    - name: RELATED_IMAGE_OPENBAO
      value: "my-registry.corp/openbao/openbao"
    - name: OPERATOR_INIT_IMAGE_REPOSITORY
      value: "my-registry.corp/openbao-init"
    - name: OPERATOR_BACKUP_IMAGE_REPOSITORY
      value: "my-registry.corp/openbao-backup"
    - name: OPERATOR_UPGRADE_IMAGE_REPOSITORY
      value: "my-registry.corp/openbao-upgrade"

provisioner:
  extraEnv:
    - name: RELATED_IMAGE_OPENBAO
      value: "my-registry.corp/openbao/openbao"
    - name: OPERATOR_INIT_IMAGE_REPOSITORY
      value: "my-registry.corp/openbao-init"
    - name: OPERATOR_BACKUP_IMAGE_REPOSITORY
      value: "my-registry.corp/openbao-backup"
    - name: OPERATOR_UPGRADE_IMAGE_REPOSITORY
      value: "my-registry.corp/openbao-upgrade"`}
>
  In multi-tenant mode, keep the controller and provisioner defaults aligned so both reconciler paths resolve helper images from the same mirrored repositories.
</CommandBlock>

<Callout type="note" title="Install defaults are not the only image contract">

Install-wide defaults are the safest starting point, but they do not replace cluster-level overrides when a specific OpenBaoCluster needs a different tag, mirror, or promotion cadence.

</Callout>

## Override images per cluster

<CommandBlock
  language="yaml"
  label="configure"
  title="Override mirrored workload images per cluster"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
  namespace: openbao
spec:
  version: "2.5.0"
  image: "my-registry.corp/openbao/openbao:2.5.0"
  imagePullSecrets:
    - name: cluster-registry-creds
  initContainer:
    image: "my-registry.corp/openbao-init:0.1.0-rc.5"
  backup:
    image: "my-registry.corp/openbao-backup:0.1.0-rc.5"
  upgrade:
    image: "my-registry.corp/openbao-upgrade:0.1.0-rc.5"`}
>
  Set explicit per-cluster images when the registry path or promotion cadence differs from the install-wide defaults. Otherwise, let the operator derive them from the mirrored repositories and the requested version.
</CommandBlock>

<CommandBlock
  language="bash"
  label="apply"
  title="Create the pull secret in the tenant namespace"
  code={`kubectl create secret docker-registry cluster-registry-creds \\
  --namespace openbao \\
  --docker-server=my-registry.corp \\
  --docker-username=<user> \\
  --docker-password=<password>`}
>
  The Secret must exist in the same namespace as the OpenBaoCluster that references it.
</CommandBlock>

<DecisionTable
  kind="reference"
  title="Disconnected-environment checks"
  columns={["Check", "What good looks like", "Why it matters"]}
  rows={[
    {
      cells: [
        "Every runtime image is mirrored",
        "Operator, OpenBao, init, backup, and upgrade images exist in the internal registry before install or rollout.",
        "A cluster that relies on public registry fallback is not disconnected-ready, even if the main OpenBao image is mirrored.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Pull secrets exist in every runtime namespace",
        "The operator namespace and every tenant namespace that runs workloads have the correct registry credential Secret.",
        "Install success does not imply workload success. Clusters can still fail to reconcile when the tenant namespace lacks the pull secret.",
      ],
    },
    {
      cells: [
        "Version and tag promotion is explicit",
        "Image tags and repository mirrors are tracked as part of the release process.",
        "Disconnected environments make silent tag drift harder to notice and more painful to debug later.",
      ],
    },
  ]}
/>

<Callout type="tip" title="Keep image verification and registry strategy separate in your head">

This page explains where images come from and how they are pulled. Signature verification, digest pinning, and trust roots are handled in the supply-chain security model, not by the mirror configuration alone.

</Callout>

<NextActions
  title="Related platform-readiness work"
  items={[
    {
      label: "Supply-chain verification",
      description: "Verify mirrored images deliberately instead of assuming an internal registry alone makes them trustworthy.",
      docId: "security/workload/supply-chain",
    },
    {
      label: "Server configuration",
      description: "Return to the cluster baseline when plugin images, audit devices, or runtime settings also need to be declared.",
      docId: "user-guide/openbaocluster/configuration/server",
    },
    {
      label: "Restore from backup",
      description: "Review restore-job image and auth assumptions before a real incident forces you to discover them under pressure.",
      docId: "user-guide/openbaorestore/restore",
    },
  ]}
/>
