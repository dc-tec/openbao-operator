---
title: Cert Manager
hide_title: true
pageType: concept
journey: architecture
description: Manage TLS certificate sources, rotation windows, and hot-reload signaling for the OpenBao workload path.
---

<PageHeader
  title="Own certificate sources, rotation, and hot reload without restarting pods."
  lede="The cert manager keeps TLS lifecycle close to the workload contract. It decides whether certificates are operator-managed, externally supplied, or handled by ACME, and it turns certificate changes into safe reload signals instead of pod restarts."
/>

<ManagerAtAGlance
  sections={[
    {
      label: 'Control path',
      items: [
        'workload reconciler',
        'internal/controller/openbaocluster',
        'internal/service/certs',
      ],
    },
    {
      label: 'Owns',
      items: [
        'TLS mode handling for OperatorManaged, External, and ACME',
        'CA and server certificate Secret lifecycle when the operator is the source',
        'certificate hash generation and hot-reload signaling when leaf certs change',
      ],
    },
    {
      label: 'Writes',
      items: [
        'cluster TLS CA and server Secrets in OperatorManaged mode',
        'trust-bundle ConfigMap surfaces for workload consumers',
        'pod annotations used to trigger in-pod TLS reload',
      ],
    },
    {
      label: 'Depends on',
      items: [
        'spec.tls.mode and rotation window settings',
        'external Secret availability when TLS is not operator-managed',
        'ready workload pods to accept reload signaling',
      ],
    },
  ]}
/>

## Architectural Placement

TLS lifecycle stays on the workload reconcile path:

1. `internal/controller/openbaocluster` determines that TLS state must reconcile.
2. The controller calls `internal/service/certs`.
3. The cert manager creates, validates, or watches certificate material and signals reload when the active server certificate changes.

That keeps TLS behavior close to workload rendering while avoiding a second, disconnected certificate controller path.

<DecisionTable
  kind="decision"
  title="TLS mode responsibilities"
  columns={['Mode', 'Certificate source', 'Manager behavior']}
  rows={[
    {
      cells: ['OperatorManaged', 'Operator-generated root CA and server certificate Secrets.', 'Generate, rotate, export trust, and trigger reload when the leaf certificate hash changes.'],
      emphasis: 'recommended',
    },
    {
      cells: ['External', 'User or external controller provided Secrets.', 'Wait for required Secrets, validate usability, and still trigger reload when the external provider rotates content.'],
    },
    {
      cells: ['ACME', 'OpenBao internal ACME flow and certificate cache.', 'Render the ACME listener configuration and step out of Secret management and hot-reload ownership.'],
      emphasis: 'caution',
    },
  ]}
/>

## Rotation And Reload Path

<DiagramFrame
  title="TLS rotation loop"
  caption="In operator-managed mode, the manager decides when the current server certificate is within the rotation window, writes fresh material, computes a new hash, and signals reload only when the hash changed."
  code={`graph TD
    Check{"Check cert expiry"} --> Generate["Generate or reuse cert material"]
    Generate --> Secrets["Update tls-ca / tls-server Secrets"]
    Secrets --> Hash["Compute server-cert hash"]
    Hash --> Change{"Hash changed?"}
    Change --> Skip["No reload signal"]
    Change --> Annotate["Annotate ready pods with new hash"]
    Annotate --> Watcher["In-pod watcher observes change"]
    Watcher --> Reload["Send SIGHUP / reload TLS"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#f8fafc;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Check read;
    class Generate,Hash,Watcher process;
    class Secrets,Annotate,Reload write;
    class Change,Skip read;`}
/>

<DiagramFrame
  title="Hot-reload boundary"
  caption="The manager does not restart pods to pick up new certificates. Instead, it updates pod-level hash annotations that a lightweight in-pod watcher turns into a reload signal."
  code={`sequenceDiagram
    participant Certs as Cert manager
    participant K8s as Kubernetes API
    participant Pod as Pod annotation + mounted volume
    participant Watcher as PID 1 watcher
    participant Bao as OpenBao

    Certs->>K8s: Update TLS Secret or confirm external change
    Certs->>K8s: Patch pod annotation openbao.org/tls-cert-hash
    K8s->>Pod: Project new Secret contents and annotation
    Pod->>Watcher: File or annotation change observed
    Watcher->>Bao: Send reload signal
    Bao->>Bao: Reload listener certificate
  `}
/>

<Callout type="note" title="ACME changes the ownership boundary">

When `spec.tls.mode=ACME`, the cert manager still participates in rendering the listener contract, but OpenBao owns the live ACME issuance and cache lifecycle. Kubernetes TLS Secrets stop being the source of truth for the workload certificate.

</Callout>

<DecisionTable
  kind="reference"
  title="Safety boundaries"
  columns={['Concern', 'Manager behavior']}
  rows={[
    {
      cells: ['No forced pod restart', 'Certificate changes become hot-reload signals instead of StatefulSet rollouts or manual restart requirements.'],
      emphasis: 'recommended',
    },
    {
      cells: ['External TLS still observed', 'External mode is passive about issuance, but active about detecting usable Secret changes and propagating reload signals.'],
    },
    {
      cells: ['Reload only on change', 'The manager computes a certificate hash and skips signaling when the new material is identical to what ready pods already observe.'],
    },
    {
      cells: ['Source-of-truth clarity', 'OperatorManaged owns Secrets, External owns watching those Secrets, and ACME owns only the rendered listener contract rather than Secret reconciliation.'],
    },
  ]}
/>

<NextActions
  title="Related deep dives"
  items={[
    {
      label: 'Infrastructure manager',
      description: 'See how TLS mode changes rendered config, volume mounts, and pod expectations in the workload path.',
      docId: 'architecture/infra-manager',
    },
    {
      label: 'TLS security guide',
      description: 'Compare the internal certificate lifecycle model with the user-facing TLS configuration guidance.',
      docId: 'security/workload/tls',
    },
    {
      label: 'External TLS validated deployment',
      description: 'See how external certificate ownership looks in a concrete hardened deployment recipe.',
      docId: 'user-guide/validated-deployments/recipes/local/hardened-transit-external-tls',
    },
  ]}
/>
