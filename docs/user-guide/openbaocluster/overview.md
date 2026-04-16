---
title: Cluster Overview
slug: /configure/cluster-overview
hide_title: true
pageType: concept
journey: configure
description: Understand what OpenBaoCluster owns, what the operator protects automatically, and how to read the Configure section as one cluster-shaping system.
---

<PageHeader
  title="OpenBaoCluster as the service contract"
  lede="`OpenBaoCluster` is the declarative contract for the running OpenBao service on Kubernetes. It sets the cluster shape, service boundary, storage path, and day 2 capabilities the operator will keep reconciling. Use this page to orient the rest of Configure and understand which areas to tune next."
/>

<Checklist
    title="A good read of this page should leave you with"
    items={[
      "a clear view of what the operator owns versus what the platform still expects from you",
      "the right mental model for spec as desired state and status as the observed operating surface",
      "a shorter list of pages to read next instead of a flat wall of configuration topics",
      "the confidence to shape the first cluster without treating the CR as an arbitrary field dump",
    ]}
  />


<Callout type="note" title="Profile is an explicit part of the contract">

Set `spec.profile` deliberately. `Hardened` and `Development` are not stylistic presets; they change the security posture, bootstrap requirements, and what the operator will allow the cluster to do.

</Callout>

<DecisionTable
  title="What OpenBaoCluster is responsible for"
  columns={["Area", "What the spec controls", "What still belongs to you"]}
  rows={[
    {
      cells: [
        "Cluster shape",
        "Version, replica count, security profile, bootstrap path, and the baseline runtime configuration the operator should keep in sync.",
        "Choose values that match the environment and failure model you actually intend to run.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Service boundary",
        "Services, Gateway or Ingress integration, and cluster network configuration.",
        "Own the edge design, DNS, certificate authority, and network assumptions outside the cluster itself.",
      ],
    },
    {
      cells: [
        "Platform readiness",
        "Storage size, workload requests, telemetry enablement, and mirrored-image defaults that affect runtime behavior.",
        "Provide the correct StorageClass, registry access, monitoring stack, and cluster capacity around the workload.",
      ],
    },
    {
      cells: [
        "Day 2 workflow hooks",
        "Backup scheduling, restore auth bootstrap, upgrade strategy, maintenance mode, and pause behavior.",
        "Actually operate those flows: verify backups, plan upgrades, and respond to recovery conditions as part of the service lifecycle.",
      ],
    },
  ]}
/>

<DiagramFrame
  title="GitOps contract"
  caption="The spec is the desired cluster shape. The operator renders and maintains the Kubernetes workload, then reports the observed operating state back on status."
  code={`flowchart LR
    Git["Git or deployment source"] --> Spec["OpenBaoCluster.spec"]
    Spec --> Operator["OpenBao Operator"]
    Operator --> Workload["StatefulSet, Services, ConfigMaps, Secrets, Jobs"]
    Workload --> Status["OpenBaoCluster.status"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Git,Spec read;
    class Operator process;
    class Workload,Status write;`}
/>

<CommandBlock
  language="yaml"
  label="configure"
  title="Recognize the major spec surfaces in one manifest"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
  namespace: openbao
spec:
  version: "2.5.0"
  profile: Hardened
  replicas: 3

  storage:
    size: "50Gi"
    storageClassName: "fast-ssd"

  selfInit:
    enabled: true
    oidc:
      enabled: true

  service:
    type: ClusterIP

  observability:
    metrics:
      enabled: true

  backup:
    schedule: "0 3 * * *"`}
>
  The point is not to memorize every field. It is to see the major configuration surfaces that map to the rest of the section: baseline, service boundary, platform readiness, and day 2 operations.
</CommandBlock>

<RouteList
  title="Read Configure in layers"
  items={[
    {
      eyebrow: "01",
      title: "Cluster baseline",
      description: "Choose the profile, bootstrap path, and server defaults that define how the cluster starts and what posture it can realistically sustain.",
      docId: "user-guide/openbaocluster/configuration/security-profiles",
    },
    {
      eyebrow: "02",
      title: "Service boundary",
      description: "Define how traffic reaches OpenBao, where TLS terminates, and which network assumptions the workload is allowed to make.",
      docId: "user-guide/openbaocluster/configuration/external-access",
    },
    {
      eyebrow: "03",
      title: "Platform readiness",
      description: "Finish storage, observability, and registry decisions before the cluster becomes expensive to move or hard to inspect.",
      docId: "user-guide/openbaocluster/configuration/resources-storage",
    },
  ]}
/>

<NextActions
  title="Continue cluster shaping"
  items={[
    {
      label: "Security profiles",
      description: "Start with the production posture and bootstrap model before you spend time on edge or observability details.",
      docId: "user-guide/openbaocluster/configuration/security-profiles",
    },
    {
      label: "External access",
      description: "Move to the service boundary once the baseline cluster shape is clear.",
      docId: "user-guide/openbaocluster/configuration/external-access",
    },
    {
      label: "Prepare for day 2",
      description: "Return to the operating path once configuration choices are explicit enough to support upgrades, backups, and recovery.",
      docId: "user-guide/openbaocluster/next-steps",
    },
  ]}
/>
