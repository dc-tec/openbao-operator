---
title: Create Your First Cluster
slug: /get-started/first-cluster
hide_title: true
description: Apply a development or hardened OpenBaoCluster profile, verify readiness, and move cleanly into day 2 operations.
---

<JourneyHero
  eyebrow="Step 3"
  title="Create the first cluster you can safely build on."
  lede="This page is the handoff from operator installation into a real OpenBaoCluster. Choose the starting profile that matches your environment, verify the cluster reaches the right readiness conditions, and then move directly into day 2 controls."
  actions={[
    {label: 'Prepare for day 2', docId: 'user-guide/openbaocluster/next-steps', variant: 'primary'},
    {label: 'Review cluster overview', docId: 'user-guide/openbaocluster/overview', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Before you apply the cluster manifest"
    items={[
      'confirm the operator is installed and healthy in the namespace model you chose',
      'onboard the target namespace through OpenBaoTenant when you are in multi-tenant mode',
      'choose a StorageClass explicitly for production before the first reconcile',
      'decide whether this cluster is development-only or intended to become production',
    ]}
  />
</JourneyHero>

<JourneySteps
  title="Cluster creation should now be predictable"
  current={3}
  items={[
    {
      label: 'Choose a deployment path',
      description: 'Decide tenancy mode, security profile, TLS posture, and install method.',
      docId: 'user-guide/operator/deployment-decision-guide',
    },
    {
      label: 'Install the operator',
      description: 'Use Helm or manifests with the right namespace, identity, and admission model.',
      docId: 'user-guide/operator/installation',
    },
    {
      label: 'Create your first cluster',
      description: 'Apply a starting profile that matches local evaluation or hardened production.',
      docId: 'user-guide/openbaocluster/getting-started',
    },
    {
      label: 'Prepare for day 2',
      description: 'Move into production checklist items, backups, exposure, and observability.',
      docId: 'user-guide/openbaocluster/next-steps',
    },
  ]}
/>

## Prerequisites

- **OpenBao Operator**: Installed and running (see [Installation](../operator/installation.md))
- **Tenancy**: In multi-tenant mode, the target namespace must be onboarded via `OpenBaoTenant` (see [Tenant Onboarding](../openbaotenant/onboarding.md)).
- **Storage Class**: A suitable StorageClass is available in the cluster. For production, prefer setting `spec.storage.storageClassName` explicitly before the first reconcile.

## Choose a starting profile

Pick the closest starting point, then adjust only the fields your environment really needs.

<Tabs groupId="development-local-testing-production">

<TabItem value="development-local-testing" label="Development (Local Testing)">

For local development and testing. **Not suitable for production.**

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: dev-cluster
  namespace: default
spec:
  version: "2.4.4"
  # image: "openbao/openbao:2.4.4" # Optional: inferred from version
  replicas: 3
  profile: Development
  tls:
    enabled: true
    mode: OperatorManaged
    rotationPeriod: "720h"
  storage:
    size: "10Gi"
```

<Callout type="warning" title="Development profile only">

The `Development` profile uses static auto-unseal and stores sensitive material in Kubernetes Secrets. This is convenient for testing but **insecure for production use**.

</Callout>

</TabItem>

<TabItem value="production" label="Production">

For production deployments with hardened security.

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
  namespace: openbao
spec:
  version: "2.4.4"
  # image: "openbao/openbao:2.4.4" # Optional: inferred from version
  replicas: 3
  profile: Hardened
  tls:
    enabled: true
    mode: External
  storage:
    size: "50Gi"
  selfInit:
    enabled: true
    oidc:
      enabled: true  # (1)!
    requests:
      # Configure user authentication FIRST to prevent lockout
      - name: enable-userpass
        operation: update
        path: sys/auth/userpass
        authMethod:
          type: userpass
          description: "Userpass authentication"
      - name: create-admin-policy
        operation: update
        path: sys/policies/acl/admin
        policy:
          policy: |
            path "*" {
              capabilities = ["create", "read", "update", "delete", "list", "sudo"]
            }
      # Then configure secret engines
      - name: enable-kv-v2
        operation: update
        path: sys/mounts/secret
        secretEngine:
          type: kv
          description: "General purpose KV store"
          options:
            version: "2"
  unseal:
    type: awskms
    awskms:
      region: us-east-1
      kmsKeyID: alias/openbao-unseal
```

1. **OIDC bootstrap** enables the Operator to authenticate via JWT for cluster lifecycle operations such as backups and upgrades. This is separate from user authentication.

<Callout type="danger" title="Lockout prevention required">

**CRITICAL**: The `requests` array **must** include user authentication configuration (for example userpass, JWT, or Kubernetes auth) before enabling self-init. OIDC bootstrap only provides Operator authentication, not user access. Enabling `selfInit.enabled: true` without user authentication in requests results in **permanent lockout** with no recovery options. See [Self-Initialization](configuration/self-init.md) for details.

</Callout>

<Callout type="tip" title="Production reminder">

Before exposing a production cluster, complete the [Production Checklist](operations/production-checklist.md).

</Callout>

</TabItem>

</Tabs>

## Apply the configuration

```sh
kubectl apply -f cluster.yaml
```

## Verify deployment

Check the cluster status:

```sh
kubectl get openbaocluster <name> -n <namespace>
```

Watch pods come up:

```sh
kubectl get pods -l openbao.org/cluster=<name> -n <namespace> -w
```

## Check status conditions

```sh
kubectl describe openbaocluster <name> -n <namespace>
```

Look for:

- `status.phase` — Current lifecycle phase
- `status.readyReplicas` — Number of ready replicas
- `status.initialized` — `true` after cluster initialization
- `status.conditions`:
  - `Available` — Cluster is serving requests
  - `TLSReady` — TLS certificates are valid
  - `ProductionReady` — Security requirements met (Hardened only)
  - `StorageConfigured` — Shows whether the effective StorageClass was explicit, defaulted, or inconsistent
  - `Degraded` — Issues detected

<OutcomePanel
  title="A healthy first cluster gives you a stable place to continue, not a fragile demo."
  tone="success"
  actions={[
    {label: 'Prepare for day 2', docId: 'user-guide/openbaocluster/next-steps'},
    {label: 'Open the production checklist', docId: 'user-guide/openbaocluster/operations/production-checklist'},
  ]}
>
  <p>Before you leave this page, confirm the basics are true:</p>

  - the cluster is `Available` and its replicas are healthy
  - TLS and storage are configured the way you intended
  - hardened clusters report `ProductionReady=True`
  - you know whether the next task is exposure, auth, backups, or production hardening
</OutcomePanel>

## What usually comes next

<CardGrid>
  <LinkCard eyebrow="Access" title="Expose the cluster" docId="user-guide/openbaocluster/configuration/external-access">
    Choose how users and workloads reach the cluster and verify the TLS mode matches your profile.
  </LinkCard>
  <LinkCard eyebrow="Security" title="Review security profiles" docId="user-guide/openbaocluster/configuration/security-profiles">
    Understand what the Development and Hardened profiles enforce before you standardize on one.
  </LinkCard>
  <LinkCard eyebrow="Recovery" title="Configure backups" docId="user-guide/openbaocluster/operations/backups">
    Wire snapshots and object storage early so restore is not first attempted during an incident.
  </LinkCard>
</CardGrid>

## Official OpenBao documentation

- [OpenBao self-initialization](https://openbao.org/docs/configuration/self-init/)
