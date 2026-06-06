---
title: Air-Gapped and Private Registries
hide_title: true
pageType: task
journey: configure
description: Mirror operator, workload, and helper images; set repository defaults; and wire pull secrets for disconnected or private-registry environments.
---

<PageHeader
  title="Air-gapped and private-registry image planning"
  lede="An air-gapped or private-registry deployment involves operator, workload, and helper executor images. Use this page to make those image defaults explicit and wire pull access for disconnected or private-registry environments."
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
        "The operator install uses chart-level `imagePullSecrets`; each cluster uses `spec.imagePullSecrets`. Image verification uses its own `imagePullSecrets` fields.",
        "Create Docker registry Secrets in the namespace that will pull the images.",
        "Do not assume the operator namespace and tenant namespaces can share pull secrets implicitly. If verification must contact a private registry, the controller also needs read access to the named tenant Secret.",
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
  tag: "<operator-version>"
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

<Callout type="note" title="Pin one published operator version across every mirrored image surface">

Use the same released operator version for the controller, provisioner, init, backup, and upgrade images. For example, once `0.1.0` is published, mirror and pin `0.1.0` consistently instead of mixing tags or treating `next` as an artifact reference.

</Callout>

<Callout type="note" title="Use install defaults and cluster overrides together">

Install-wide defaults provide the baseline image contract. Use cluster-level overrides when a specific `OpenBaoCluster` needs a different tag, mirror, or promotion cadence.

</Callout>

<Callout type="warning" title="Per-cluster helper image overrides need delegated RBAC">

Setting `spec.initContainer.image`, `spec.backup.image`, or `spec.upgrade.image` chooses executables that run with operator-managed mounts, credentials, or job identities. The human or GitOps identity applying that `OpenBaoCluster` needs the `usecustomexecutables` verb on the cluster. Existing `usehelperimages` grants continue to work as a compatibility alias.

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
  imageVerification:
    enabled: true
    imagePullSecrets:
      - name: cluster-registry-creds
  operatorImageVerification:
    enabled: true
    imagePullSecrets:
      - name: cluster-registry-creds
  initContainer:
    image: "my-registry.corp/openbao-init:<operator-version>"
  backup:
    image: "my-registry.corp/openbao-backup:<operator-version>"
  upgrade:
    image: "my-registry.corp/openbao-upgrade:<operator-version>"`}
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

<Callout type="note" title="Separate kubelet pulls from verification pulls">

`spec.imagePullSecrets` lets Kubernetes pull the rendered Pods and Jobs. `spec.imageVerification.imagePullSecrets` and `spec.operatorImageVerification.imagePullSecrets` let the controller authenticate to the registry while resolving and verifying signatures. In multi-tenant mode, the provisioner grants the controller `get` only on those named tenant Secrets.

</Callout>

<DecisionTable
  kind="reference"
  title="Disconnected-environment checks"
  columns={["Check", "Expected state", "Why it matters"]}
  rows={[
    {
      cells: [
        "Every runtime image is mirrored",
        "Operator, OpenBao, init, backup, and upgrade images exist in the internal registry before install or rollout.",
        "Disconnected operation requires every runtime image to be mirrored, not only the main OpenBao image.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Pull secrets exist in every runtime namespace",
        "The operator namespace and every tenant namespace that runs workloads have the correct registry credential Secret.",
        "Install success and workload reconcile success are separate checks. Tenant namespaces still need the correct pull secret.",
      ],
    },
    {
      cells: [
        "Version and tag promotion is explicit",
        "Image tags and repository mirrors are tracked as part of the release process.",
        "Disconnected environments require explicit tag promotion records because drift is harder to detect later.",
      ],
    },
  ]}
/>

<Callout type="tip" title="Keep image verification and registry strategy separate in your head">

Use this guide to understand where images come from and how they are pulled. Signature verification, digest pinning, and trust roots are handled in the supply-chain security model, not by the mirror configuration alone.

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
