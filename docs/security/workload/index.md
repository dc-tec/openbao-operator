---
title: Workload Protections
hide_title: true
pageType: landing
journey: security
description: Workload security guidance for OpenBao Operator covering pod security defaults, TLS protections, and software supply chain verification.
---

<PageHero
  variant="landing"
  eyebrow="Security / Workload Protections"
  title="Workload security controls"
  lede="Pod and container hardening, workload identity and TLS, and image verification."
  actions={[
    {label: 'Open pod and runtime security', docId: 'security/workload/workload-security', variant: 'primary'},
    {label: 'Review TLS and identity', docId: 'security/workload/tls', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Covers"
    items={[
      'verify the default pod hardening posture',
      'understand how server TLS, peer identity, and trust material are managed',
      'decide how image verification should work in production',
      'review runtime protections separately from RBAC and admission policy concerns',
    ]}
  />
</PageHero>

<RouteList
  title="Workload protection routes"
  items={[
    {
      eyebrow: '01',
      title: 'Pod and runtime security',
      description: 'Pod security context, filesystem, token, and container-hardening defaults.',
      docId: 'security/workload/workload-security',
    },
    {
      eyebrow: '02',
      title: 'TLS and identity',
      description: 'Server TLS, peer trust, certificate management, and workload-facing identity paths.',
      docId: 'security/workload/tls',
    },
    {
      eyebrow: '03',
      title: 'Supply-chain verification',
      description: 'Digest pinning, signature verification, and image-trust guardrails.',
      docId: 'security/workload/supply-chain',
    },
  ]}
/>

<Callout type="note" title="Default runtime hardening">

OpenBao Pods are expected to run non-root with a read-only root filesystem, dropped Linux capabilities, and a `RuntimeDefault` seccomp profile. The detailed page should explain exceptions and platform dependencies, not re-argue the baseline.
OpenBao Pods are expected to run non-root with a read-only root filesystem, dropped Linux capabilities, and a `RuntimeDefault` seccomp profile. The detailed page covers exceptions and platform dependencies.

</Callout>

<NextActions
  items={[
    {
      label: 'Open platform controls',
      description: 'How runtime protections connect to RBAC, admission, and network boundaries.',
      docId: 'security/infrastructure/index',
    },
    {
      label: 'Review production posture',
      description: 'Tie workload protections back to the Hardened production contract.',
      docId: 'security/fundamentals/profiles',
    },
  ]}
/>
