---
title: Secrets and Trust Material
hide_title: true
pageType: concept
journey: security
description: How root tokens, unseal keys, TLS material, and generated job identities are created, avoided, or bounded across the OpenBao Operator lifecycle.
---

<PageHero
  variant="compact"
  eyebrow="Security / Security Model"
  title="Treat trust material as lifecycle state, not just as Kubernetes Secrets."
  lede="The operator manages or coordinates several high-value trust surfaces: bootstrap credentials, unseal roots, TLS material, and the identities used by backup, restore, and upgrade workflows. The most important question is not only where they live, but whether the operating model can avoid creating them in the first place."
  actions={[
    {label: 'Open production posture', docId: 'security/fundamentals/profiles', variant: 'primary'},
    {label: 'Open backup operations', docId: 'user-guide/openbaocluster/operations/backups', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Use this page when you need to"
    items={[
      'understand which trust material the operator may create or avoid',
      'decide whether static unseal or persisted bootstrap secrets are acceptable',
      'review how backup, restore, and upgrade jobs authenticate',
      'connect lifecycle workflows back to the trust model instead of treating them as isolated features',
    ]}
  />
</PageHero>

<DecisionTable
  kind="reference"
  title="Managed trust material"
  columns={['Surface', 'Typical location', 'Security posture']}
  rows={[
    {
      cells: [
        'Root token',
        'Secret only when bootstrap mode persists it',
        'Critical and intentionally avoided by the supported Hardened self-init path.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Static unseal key',
        '`<cluster>-unseal-key` Secret',
        'Critical because the root of trust sits inside Kubernetes instead of an external trust system.',
      ],
    },
    {
      cells: [
        'Cluster CA and server TLS material',
        'Secrets when the selected TLS mode requires them',
        'High-value trust material whose lifecycle depends on the chosen TLS mode.',
      ],
    },
    {
      cells: [
        'Backup, restore, and upgrade job auth',
        'Projected tokens, generated ServiceAccounts, or explicit credentials Secrets',
        'Should remain separate from the main OpenBao workload identity.',
      ],
    },
  ]}
/>

## Unseal trust model

<Tabs groupId="trust-material-unseal">

<TabItem value="static" label="Static unseal">

<Callout type="warning" title="Static unseal keeps the root of trust in the cluster">

Static unseal generates a 32-byte key and stores it in a Kubernetes Secret. If etcd encryption or namespace access is weak, the effective trust root of your OpenBao data is weak too.

</Callout>

<DecisionTable
  kind="reference"
  title="Static unseal behavior"
  columns={['Aspect', 'Behavior', 'Why it matters']}
  rows={[
    {
      cells: [
        'Generation',
        'The operator creates a random key.',
        'Bootstrap is convenient, but the cluster is now trusted to protect the root of trust.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Storage',
        'The key is stored in `<cluster>-unseal-key`.',
        'Anyone who can read that Secret can compromise the trust root.',
      ],
    },
    {
      cells: [
        'Rotation',
        'Manual, not automatic.',
        'Operational handling of the trust root stays a human responsibility.',
      ],
    },
  ]}
/>

</TabItem>

<TabItem value="external" label="External trust root">

<Callout type="success" title="Preferred production posture">

Using transit, cloud KMS, or HSM-backed modes shifts the root of trust away from Kubernetes and into an external system that is designed to protect it.

</Callout>

<DecisionTable
  kind="reference"
  title="External trust behavior"
  columns={['Aspect', 'Behavior', 'Why it matters']}
  rows={[
    {
      cells: [
        'Operator-owned unseal key',
        'Not created',
        'The cluster does not persist its own trust root in a Kubernetes Secret.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Authentication path',
        'Uses workload identity or explicit credentials for the selected backend.',
        'The identity contract becomes part of the production posture and must be observable.',
      ],
    },
    {
      cells: [
        'Readiness model',
        'Surfaced through conditions such as `CloudUnsealIdentityReady`.',
        'Failures become visible before operators have to infer them from generic pod errors.',
      ],
    },
  ]}
/>

</TabItem>

</Tabs>

## Bootstrap credentials and root token handling

<DecisionTable
  title="Bootstrap paths"
  columns={['Path', 'What happens to the root token', 'Security effect']}
  rows={[
    {
      cells: [
        'Hardened self-init',
        'The initial root token is not persisted as the normal operating model.',
        'This avoids leaving a long-lived administrative credential in namespace Secrets.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Development bootstrap without self-init',
        'The root token can be stored in `<cluster>-root-token`.',
        'This is useful for testing but creates a critical secret in the namespace.',
      ],
    },
  ]}
/>

<Callout type="danger" title="Persisted bootstrap secrets are full-administration material">

If a root token or equivalent bootstrap credential exists in a Secret, anyone who can read that Secret effectively has full administrative control of the OpenBao cluster.

</Callout>

## TLS and job identities

<DiagramFrame
  title="Job identity separation"
  caption="The main OpenBao Pods, backup jobs, restore jobs, and upgrade jobs should not silently share one identity path. Each surface should stay explicit and observable."
  code={`sequenceDiagram
    autonumber
    participant Operator as Operator
    participant K8s as Kubernetes API
    participant Bao as OpenBao
    participant Job as Lifecycle Job

    Note over Operator,K8s: Discovery
    Operator->>K8s: Discover OIDC issuer and JWKS

    Note over Operator,Bao: Bootstrap
    Operator->>Bao: Configure JWT auth and roles

    Note over Operator,Job: Execution
    Operator->>K8s: Create Job with generated ServiceAccount
    K8s-->>Job: Start with projected token
    Job->>Bao: Login with JWT
    Bao-->>Job: Scoped OpenBao token
    Job->>Bao: Perform backup, restore, or upgrade work`}
/>

The important boundary is this:

- main OpenBao Pods use the trust path selected for the cluster itself
- backup, restore, and upgrade Jobs use separate generated identities
- those Jobs do not automatically inherit the cloud or JWT path of the main workload unless the operator deliberately configured it

This is why backup and restore readiness are surfaced independently in status rather than assumed from the main Pods.

## Where the task guidance lives

This page owns the trust model. The operational task pages stay elsewhere:

- <SiteLink docId="user-guide/openbaocluster/configuration/security-profiles">Configure Security Profiles</SiteLink>
- <SiteLink docId="user-guide/openbaocluster/operations/backups">Configure Backups</SiteLink>
- <SiteLink docId="user-guide/openbaorestore/restore">Restore from Backup</SiteLink>

<NextActions
  title="Continue the security model"
  items={[
    {
      label: 'Production posture',
      description: 'See how these trust-material choices map back to Development versus Hardened.',
      docId: 'security/fundamentals/profiles',
    },
    {
      label: 'Configure security profiles',
      description: 'Switch to the task page when you are ready to set the actual cluster fields.',
      docId: 'user-guide/openbaocluster/configuration/security-profiles',
    },
    {
      label: 'Threat model',
      description: 'Return to the broader threat model if you need the surrounding attacker and boundary assumptions.',
      docId: 'security/fundamentals/threat-model',
    },
  ]}
/>
