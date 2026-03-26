---
title: Troubleshoot the Cluster
description: Route common failure symptoms to the right fix quickly and know when to switch from troubleshooting into recovery.
slug: /operate/troubleshooting
hide_title: true
pageType: task
journey: operate
---

<PageHeader
  title="Start with the symptom, then decide whether you need a fix or a recovery path."
  lede="Most day 2 incidents start as configuration drift, edge integration failures, or capability mismatches. Use this page to capture the failing surface, route the symptom to the right fix, and escalate into recovery only when normal convergence is no longer realistic."
/>

<CommandBlock
  language="bash"
  label="inspect"
  title="Capture the failure surface first"
  code={`kubectl describe openbaocluster <name> -n <namespace>
kubectl get pods -n <namespace>
kubectl describe pod <pod> -n <namespace>`}
>
  Start here before you jump into a fix. The cluster status, recent events, and the specific pod failures usually tell you whether the problem is inside the workload, at the edge, or in cluster policy.
</CommandBlock>

<DecisionTable
  title="Choose the first troubleshooting route"
  columns={['Symptom', 'Start here', 'Likely surface', 'Escalate when']}
  rows={[
    {
      cells: [
        'Probe failures with x509 or hostname mismatch',
        'Check the service hostname, SNI source, and the SANs on the serving certificate.',
        'TLS and probe configuration.',
        'Pods still cannot become Ready after the hostname and certificate path are corrected.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'ACME challenge or join failures',
        'Check DNS, SAN coverage, and whether the chosen ACME endpoint is actually reachable from the right network.',
        'ACME domain planning or private/public edge reachability.',
        'The cluster cannot issue or validate certs and the workload remains blocked behind the edge.',
      ],
    },
    {
      cells: [
        'Gateway integration stays degraded',
        'Inspect passthrough mode, Gateway listener compatibility, and Gateway programming state.',
        'Gateway API integration.',
        'Traffic still cannot reach the workload after the Gateway listener model is corrected.',
      ],
    },
    {
      cells: [
        'Kubernetes API calls fail in a hardened cluster',
        'Check `apiServerCIDR`, `apiServerEndpointIPs`, and the actual egress behavior of your CNI.',
        'Cluster network policy and API egress.',
        'The operator cannot reconcile core resources because it cannot talk to the Kubernetes API reliably.',
      ],
    },
    {
      cells: [
        'Node hardening or capability mismatch',
        'Check AppArmor and other workload hardening assumptions against the cluster capabilities.',
        'Node capability mismatch.',
        'The runtime cannot support the hardened profile you selected and the workload cannot start cleanly.',
      ],
    },
  ]}
/>

<RouteList
  title="Common failure routes"
  items={[
    {
      title: 'Probe TLS failures',
      description: 'Readiness or liveness probes fail because the serving cert does not match the name the probe actually uses.',
      actionLabel: 'Open',
      to: '#probe-tls-failures',
    },
    {
      title: 'ACME domain and reachability',
      description: 'Certificates do not issue, the private ACME name does not resolve, or the public CA cannot hit the endpoint you exposed.',
      actionLabel: 'Open',
      to: '#acme-domain-and-reachability',
    },
    {
      title: 'Gateway passthrough mismatch',
      description: 'The Gateway listener mode or controller capability does not match the TLS mode the cluster expects.',
      actionLabel: 'Open',
      to: '#gateway-passthrough-mismatch',
    },
    {
      title: 'Kubernetes API egress',
      description: 'The hardened network policy path blocks the controller or workload from reaching the API server.',
      actionLabel: 'Open',
      to: '#kubernetes-api-egress',
    },
    {
      title: 'Node hardening mismatch',
      description: 'AppArmor or another hardening requirement is not available on the target cluster.',
      actionLabel: 'Open',
      to: '#node-hardening-mismatch',
    },
    {
      title: 'Switch to recovery',
      description: 'Normal troubleshooting is no longer enough because leadership, restore, or rollback paths are now in play.',
      actionLabel: 'Open',
      to: '#switch-to-recovery',
    },
  ]}
/>

## Probe TLS failures

### Symptom

Pod events show probe failures such as `x509: cannot validate certificate for 127.0.0.1 because it doesn't contain any IP SANs`.

### What it usually means

The serving certificate includes DNS SANs only, but the probe is connecting through loopback or another name the cert does not cover.

### Check first

- `spec.gateway.hostname`
- `spec.ingress.host`
- whether an external Service exists
- whether the selected hostname is present in the certificate SANs

### Fix

Make sure the operator has a real hostname it can use for probe SNI and that the certificate includes that hostname. If the certificate comes from an external PKI, reissue it with the correct SAN set instead of weakening probe verification.

## ACME domain and reachability

### Private ACME CA does not resolve

If `ConditionDegraded=True` with reason `ACMEDomainNotResolvable`, the configured `spec.tls.acme.domains` likely does not resolve from cluster DNS.

Use an internal domain such as `<cluster>-acme.<namespace>.svc` for private in-cluster ACME issuers and make sure CoreDNS understands any non-`.svc` override you chose for local clusters.

### ACME join or certificate verification fails

If Raft join shows `certificate signed by unknown authority` or server-name mismatch errors:

- make sure `spec.tls.acme.domains` contains names that are actually present in the issued cert
- for private ACME CAs, set `spec.configuration.acmeCARoot`
- mount `pki-ca.crt` alongside that root so the operator can use it for `retry_join` and probe verification

### Public ACME cannot reach the endpoint

If the logs show `Timeout during connect`, `secondary validation`, or repeated `tls-alpn-01` failures:

- expose the hardened hostname on a dedicated public passthrough listener on port `443`
- do not source-restrict the ACME validation path to a single client IP
- keep restricted admin edges separate from the public challenge edge if you need both

## Gateway passthrough mismatch

If `ConditionDegraded=True` with reason `ACMEGatewayNotConfiguredForPassthrough`, or `GatewayIntegrationReady=False` with reasons such as `GatewayListenerIncompatible`, `GatewayFeatureUnsupported`, or `GatewayNotProgrammed`, start at the Gateway listener model.

For `tls.mode: ACME`:

- set `spec.gateway.tlsPassthrough: true`
- make sure the referenced Gateway has a `TLS` listener with `tls.mode: Passthrough`
- verify the Gateway controller actually supports the required route feature

TLS termination at the Gateway breaks OpenBao's ability to complete ACME challenges. Fix the passthrough model first instead of debugging the workload Pods.

## Kubernetes API egress

If `APIServerNetworkReady=False` with reason `APIServerNetworkConfigurationInvalid`, or if API calls fail in a hardened cluster:

- set `spec.network.apiServerCIDR` when the in-cluster service VIP cannot be discovered or you want an explicit allow-list
- add `spec.network.apiServerEndpointIPs` when your CNI enforces egress on post-DNAT traffic and the service-VIP path still fails
- treat `APIServerEndpointIPsRecommended` as advice, not automatically as a hard failure

<CommandBlock
  language="bash"
  label="inspect"
  title="Inspect the current API-network conditions"
  code={`kubectl get openbaocluster <name> -n <namespace> -o jsonpath='{range .status.conditions[*]}{.type}={.status}{"\\t"}{.reason}{"\\n"}{end}'`}
>
  This gives you the condition reasons in one pass so you can see whether the controller is failing on service-VIP discovery, endpoint-IP guidance, or a more direct API connectivity problem.
</CommandBlock>

## Node hardening mismatch

If `ConditionNodeSecurityCapabilityMismatch=True` with reason `AppArmorUnsupported`, the hardened profile expects a node capability your environment does not provide.

For evaluation or constrained dev clusters only, you can disable AppArmor:

<CommandBlock
  language="yaml"
  label="configure"
  title="Disable AppArmor for unsupported dev clusters"
  code={`spec:
  workloadHardening:
    appArmorEnabled: false`}
>
  Do not use this as a quiet production workaround. The right production fix is to run on nodes that satisfy the hardening profile you selected.
</CommandBlock>

## Switch to recovery

Normal troubleshooting stops being the right tool when the incident is no longer about a single configuration defect and is now about getting the service back into a safe known state.

<DecisionTable
  kind="reference"
  title="When to move from troubleshooting into recovery"
  columns={['Trigger', 'Go next', 'Why']}
  rows={[
    {
      cells: [
        'The cluster cannot elect or keep a leader',
        'No-leader recovery',
        'This is no longer just a config issue. You need a controlled Raft recovery path.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'The cluster remains sealed and the normal unseal path is not recovering it',
        'Sealed-cluster recovery',
        'The incident has crossed into availability recovery, not ordinary convergence repair.',
      ],
    },
    {
      cells: [
        'A blue-green or rollback path failed and the controller needs operator action',
        'Failed rollback recovery',
        'Version change failure modes need a dedicated incident path to avoid making the situation worse.',
      ],
    },
    {
      cells: [
        'The fastest safe recovery is to restore from a snapshot',
        'Restore operations',
        'You are now operating a destructive recovery workflow and should stop iterating on local fixes.',
      ],
    },
  ]}
/>

## External references

- [TCP listener configuration](https://openbao.org/docs/configuration/listener/tcp/)
- [ACME TLS listener RFC](https://openbao.org/docs/rfcs/acme-tls-listeners/)
- [Operator Raft command](https://openbao.org/docs/commands/operator/raft/)

<NextActions
  title="Escalate or go deeper"
  items={[
    {
      label: 'Open recovery and restore',
      description: 'Use the incident-focused recovery pages when normal troubleshooting no longer reduces risk.',
      docId: 'user-guide/openbaocluster/recovery/index',
    },
    {
      label: 'Review backup operations',
      description: 'Confirm the snapshot path is healthy before you rely on restore as part of the incident response.',
      docId: 'user-guide/openbaocluster/operations/backups',
    },
    {
      label: 'Review network security',
      description: 'Go deeper on hardened egress rules and API-server access when the incident is rooted in cluster policy.',
      docId: 'security/infrastructure/network-security',
    },
  ]}
/>
