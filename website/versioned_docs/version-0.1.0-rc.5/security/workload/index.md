---
title: Workload Protections
hide_title: true
pageType: landing
journey: security
description: Workload security guidance for OpenBao Operator covering pod security defaults, TLS protections, and software supply chain verification.
---

<PageHero
  eyebrow="Security / Workload Protections"
  title="Treat pod hardening, TLS, and image trust as one runtime surface."
  lede="Workload protections cover the controls that apply once the cluster is allowed to run: pod and container hardening, workload identity and TLS, and the supply-chain rules that decide which images the operator will trust."
  actions={[
    {label: 'Open pod and runtime security', docId: 'security/workload/workload-security', variant: 'primary'},
    {label: 'Review TLS and identity', docId: 'security/workload/tls', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Use this section when you need to"
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
      description: 'Review pod security context, filesystem, token, and container-hardening defaults.',
      docId: 'security/workload/workload-security',
    },
    {
      eyebrow: '02',
      title: 'TLS and identity',
      description: 'Understand server TLS, peer trust, certificate management, and workload-facing identity paths.',
      docId: 'security/workload/tls',
    },
    {
      eyebrow: '03',
      title: 'Supply-chain verification',
      description: 'Review digest pinning, signature verification, and the production guardrails around image trust.',
      docId: 'security/workload/supply-chain',
    },
  ]}
/>

<Callout type="note" title="Default runtime hardening">

OpenBao Pods are expected to run non-root with a read-only root filesystem, dropped Linux capabilities, and a `RuntimeDefault` seccomp profile. The detailed page should explain exceptions and platform dependencies, not re-argue the baseline.

</Callout>

<NextActions
  items={[
    {
      label: 'Open platform controls',
      description: 'Connect runtime protections back to RBAC, admission, and network boundaries.',
      docId: 'security/infrastructure/index',
    },
    {
      label: 'Review production posture',
      description: 'Tie workload protections back to the Hardened production contract.',
      docId: 'security/fundamentals/profiles',
    },
  ]}
/>
