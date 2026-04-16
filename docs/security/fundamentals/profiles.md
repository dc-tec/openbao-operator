---
title: Production Posture
hide_title: true
pageType: concept
journey: security
description: What Development and Hardened mean as security contracts, and why Hardened is the supported production posture for OpenBao Operator.
---

<PageHeader
  title="Security profile contracts"
  lede="This page explains what `Development` and `Hardened` optimize for, what Hardened requires in production, and how the two profiles change the operating contract."
/>



<DecisionTable
  title="Profile intent"
  columns={['Profile', 'Optimized for', 'What it trades off']}
  rows={[
    {
      cells: [
        'Hardened',
        'Production deployments with explicit external trust roots and stricter lifecycle guarantees.',
        'More up-front requirements, less tolerance for weak bootstrap and trust shortcuts.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Development',
        'Local testing, CI, and quick evaluation when long-term trust posture is not the goal.',
        'Allows bootstrap and unseal material to exist in cluster Secrets and relaxes some runtime guarantees.',
      ],
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Profile comparison"
  columns={['Feature', 'Hardened', 'Development']}
  rows={[
    {
      cells: [
        'Root token handling',
        'Auto-revoked; not stored in a Secret as part of the supported production path.',
        'Can be stored in a Secret when self-init is disabled.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Unseal root of trust',
        'Requires a non-static external trust source such as transit, cloud KMS, or HSM-backed modes.',
        'Defaults can rely on a static key in a Kubernetes Secret.',
      ],
    },
    {
      cells: [
        'TLS posture',
        'Requires `External` or `ACME` style trust; `OperatorManaged` TLS is not the production path.',
        'Allows operator-managed TLS for local or test usage.',
      ],
    },
    {
      cells: [
        'Bootstrap model',
        'Self-init is the supported production path.',
        'Manual bootstrap or self-init are both possible.',
      ],
    },
    {
      cells: [
        'Supply-chain guardrails',
        'Image verification protections remain on and cannot degrade to warning-only behavior.',
        'Verification can be relaxed for testing.',
      ],
    },
  ]}
/>

## Why Hardened is the supported production contract

<DecisionTable
  kind="reference"
  title="Hardened production requirements"
  columns={['Requirement', 'Why it exists', 'Operational effect']}
  rows={[
    {
      cells: [
        'At least three replicas',
        'Raft needs quorum and safe disruption handling in production.',
        'Single-node clusters are not treated as production-safe.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'External trust root for unseal',
        'The cluster should not keep its root of trust only in Kubernetes etcd.',
        'Cloud KMS, transit, KMIP, or PKCS#11-style paths become part of the deployment contract.',
      ],
    },
    {
      cells: [
        'Trusted TLS path',
        'The workload identity boundary should be anchored in a production-grade certificate model.',
        '`OperatorManaged` TLS is rejected for Hardened clusters.',
      ],
    },
    {
      cells: [
        'Self-init enabled',
        'The supported bootstrap path should avoid persisting the initial root token.',
        'Human bootstrap becomes declarative instead of secret-based.',
      ],
    },
    {
      cells: [
        'Verification guardrails stay enforced',
        'Production image trust should not be optional.',
        'Managed workloads keep digest and verification enforcement even when omitted from config.',
      ],
    },
  ]}
/>

<Callout type="success" title="What Hardened is really saying">

Hardened means the operator can rely on an external trust root, explicit runtime identity, and a production-ready lifecycle posture. It defines the operating model for the cluster, not just one field in the CR.

</Callout>

## What Development deliberately relaxes

Development is still useful, but it should be understood as an intentional weakening of the production contract:

- bootstrap material can persist in cluster Secrets
- static unseal remains available
- operator-managed TLS can be used
- runtime and supply-chain controls can be less strict
- the cluster reports a security-risk signal rather than pretending this posture is production-ready

<Callout type="warning" title="Do not upgrade trust roots in place by assumption">

Teams often start in Development for exploration. When moving to staging or production, create a new Hardened cluster rather than assuming a Development trust path can be promoted safely.

</Callout>

## Where configuration belongs

This page explains the contract. The actual task of setting `spec.profile`, choosing the unseal mode, and satisfying the production requirements belongs in <SiteLink docId="user-guide/openbaocluster/configuration/security-profiles">Configure Security Profiles</SiteLink>.

<NextActions
  title="Continue the security model"
  items={[
    {
      label: 'Configure security profiles',
      description: 'Switch to the task page when you are ready to apply the profile to a real cluster.',
      docId: 'user-guide/openbaocluster/configuration/security-profiles',
    },
    {
      label: 'Secrets and trust material',
      description: 'Review how root tokens, unseal keys, and job identities differ between these profiles.',
      docId: 'security/fundamentals/secrets-management',
    },
    {
      label: 'Threat model',
      description: 'Go back to the broader threat model if you need the rationale behind these profile boundaries.',
      docId: 'security/fundamentals/threat-model',
    },
  ]}
/>
