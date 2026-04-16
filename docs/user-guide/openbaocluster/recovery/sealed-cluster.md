---
title: Recover a Sealed Cluster
description: Separate seal, trust, identity, and reachability failures so you can restore service before moving into deeper quorum recovery.
slug: /recover/sealed-cluster
hide_title: true
pageType: runbook
journey: operate
---

<PageHeader
  title="Diagnose a sealed cluster"
  lede="A sealed cluster usually means the Pods can start but cannot complete the configured trust or unseal path they need to serve traffic. Use this runbook to start with operator-visible conditions, then narrow the problem by seal mode and move to emergency manual unseal only if needed."
/>

<Checklist
    title="Use this runbook when"
    items={[
      'Pods are running but remain sealed and not ready',
      'the cluster reports `OpenBaoSealed=True`',
      'cloud KMS, transit, TLS, or static-key dependencies might be blocking startup',
      'you need to decide whether this is a seal problem or a broader quorum problem',
    ]}
  />


<DecisionTable
  title="Read the first conditions"
  columns={['Condition or signal', 'What it usually means', 'Where to look next']}
  rows={[
    {
      cells: [
        '`OpenBaoSealed=True` while Pods are running',
        'The workload is up far enough to report status, but the unseal path is still blocked.',
        'Check the configured seal mode and the corresponding credentials or trust material.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        '`CloudUnsealIdentityReady=False`',
        'The workload identity or cloud credentials for a cloud KMS backend are not usable.',
        'Inspect the identity binding, IAM policy, and KMS reachability.',
      ],
    },
    {
      cells: [
        '`TLSReady=False`',
        'The cluster may not trust the configured certificates or may be missing required TLS material.',
        'Inspect the rendered TLS Secrets and pod logs for `x509` errors.',
      ],
    },
    {
      cells: [
        'The cluster unseals but still does not become active.',
        'This may no longer be a seal problem.',
        <>Move to <SiteLink docId="user-guide/openbaocluster/recovery/no-leader">Recover from No Leader</SiteLink>.</>,
      ],
    },
  ]}
/>

<DiagramFrame
  title="Sealed-cluster triage"
  caption="Confirm the cluster is actually sealed, identify the configured seal mode, then narrow the failure to credentials, trust, or network before using any emergency manual path."
  code={`flowchart TD
    Start["Pods running but sealed"] --> Conditions["Check operator conditions"]
    Conditions --> Mode{"Which seal mode?"}
    Mode -- "Static" --> Static["Verify Secret exists and key name is correct"]
    Mode -- "Transit" --> Transit["Verify transit auth, TLS, and network path"]
    Mode -- "Cloud KMS" --> KMS["Verify workload identity, IAM, and endpoint reachability"]
    Mode -- "KMIP / PKCS#11" --> External["Verify device, library, certs, and connectivity"]
    Static --> Healthy{"Unseals?"}
    Transit --> Healthy
    KMS --> Healthy
    External --> Healthy
    Healthy -- "No" --> Manual["Manual unseal only for emergency access"]
    Healthy -- "Yes, but no leader" --> Leader["Switch to no-leader recovery"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Start,Conditions,Mode read;
    class Static,Transit,KMS,External process;
    class Manual,Leader write;`}
/>

## Inspect the operator-visible state first

<CommandBlock
  language="bash"
  label="inspect"
  title="Read the current conditions and seal mode"
  code={`kubectl get openbaocluster <name> -n <namespace> \\
  -o jsonpath='{range .status.conditions[*]}{.type}={.status} {.reason}{"\\n"}{end}'
kubectl get openbaocluster <name> -n <namespace> -o yaml | yq '.spec.unseal'
kubectl logs -n <namespace> <pod-name> | grep -i unseal`}
>
  Focus on `OpenBaoSealed`, `CloudUnsealIdentityReady`, and `TLSReady`. These usually tell you whether the next step is credentials, trust, or network rather than generic application debugging.
</CommandBlock>

## Diagnose by seal mode

<Tabs groupId="sealed-cluster-diagnostics">

<TabItem value="static" label="Static">

Use this path when the cluster reads its unseal key from a Kubernetes Secret.

<CommandBlock
  language="bash"
  label="inspect"
  title="Verify the static unseal Secret"
  code={`kubectl get secret -n <namespace> <cluster-name>-unseal-key
kubectl get secret -n <namespace> <cluster-name>-unseal-key -o jsonpath='{.data}'`}
>
  The Secret must exist and use the expected key name `key`.
</CommandBlock>

<CommandBlock
  language="bash"
  label="apply"
  title="Create or replace the static unseal Secret"
  code={`kubectl create secret generic <cluster-name>-unseal-key -n <namespace> \\
  --from-literal=key=<UNSEAL_KEY> \\
  --dry-run=client -o yaml | kubectl apply -f -`}
/>

</TabItem>

<TabItem value="transit" label="Transit">

Use this path when the cluster unseals through another OpenBao deployment.

<DecisionTable
  title="Transit-specific failure signals"
  columns={['Signal', 'Likely cause', 'Fix first']}
  rows={[
    {
      cells: [
        '`permission denied` or auth failures',
        'The token, auth path, or transit policy is wrong.',
        'Replace the credentials Secret and verify the transit-side role or policy.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        '`x509` or trust-chain errors',
        'The transit CA or client certificate material does not match the endpoint.',
        'Reconcile the Secret contents and referenced TLS file paths.',
      ],
    },
    {
      cells: [
        '`context deadline exceeded`',
        'The transit endpoint is not reachable from the workload.',
        'Check DNS, egress rules, and the remote endpoint health.',
      ],
    },
  ]}
/>

</TabItem>

<TabItem value="cloud-kms" label="Cloud KMS">

Use this path for AWS KMS, GCP Cloud KMS, Azure Key Vault, or OCI KMS unseal backends.

<CommandBlock
  language="bash"
  label="inspect"
  title="Inspect cloud-unseal failures in the logs"
  code={`kubectl logs -n <namespace> <pod-name> | grep -Ei 'unseal|kms|decrypt|accessdenied|forbidden|timeout'`}
/>

<DecisionTable
  title="Cloud KMS failure patterns"
  columns={['Log or condition', 'Likely cause', 'Fix first']}
  rows={[
    {
      cells: [
        '`CloudUnsealIdentityReady=False` or `AccessDenied`',
        'The workload identity or IAM policy is not allowed to decrypt.',
        'Fix the ServiceAccount binding and grant the decrypt permission on the configured key.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        '`context deadline exceeded`',
        'The cluster cannot reach the KMS endpoint.',
        'Check egress rules, proxy behavior, firewall policy, and DNS.',
      ],
    },
    {
      cells: [
        'Provider-side `5xx` errors',
        'The KMS service itself may be degraded.',
        'Confirm regional health and retry only when the upstream service is stable.',
      ],
    },
  ]}
/>

</TabItem>

<TabItem value="kmip-pkcs11" label="KMIP / PKCS#11">

Use this path for external HSM or KMIP-backed unseal modes.

Check these first:

- referenced client certificate, key, and CA material
- library and device mount paths for `pkcs11`
- network reachability to the KMIP endpoint
- rendered seal configuration inside the Pod

These modes do not report `CloudUnsealIdentityReady`, so pod logs and rendered configuration are the primary signal surface.

</TabItem>

<TabItem value="manual" label="Manual emergency">

<Callout type="danger" title="Emergency only">

Manual unseal is the escape hatch when automation is broken and you need immediate access. It does not fix the underlying seal path. Use it to regain access, then repair the actual trust or credential dependency.

</Callout>

<CommandBlock
  language="bash"
  label="apply"
  title="Manually unseal a Pod"
  code={`kubectl exec -n <namespace> -it <pod-name> -- sh
bao operator unseal`}
>
  Repeat this on every Pod that needs to join the active cluster. If the cluster then stays sealed again after restart, return to the relevant automated seal mode and fix it there.
</CommandBlock>

</TabItem>

</Tabs>

## Verify the cluster is actually serving again

<CommandBlock
  language="bash"
  label="verify"
  title="Check seal status and cluster readiness"
  code={`kubectl get openbaocluster <name> -n <namespace>
kubectl exec -n <namespace> -it <pod-name> -- bao status`}
/>

If the cluster unseals but only reaches standby state or still cannot elect a leader, move to <SiteLink docId="user-guide/openbaocluster/recovery/no-leader">Recover from No Leader</SiteLink>.

<NextActions
  title="Continue with the right recovery path"
  items={[
    {
      label: 'Unseal configuration',
      description: 'Return to the exact provider and Secret contract when the incident came from wrong credential shape or mounted file paths.',
      docId: 'user-guide/openbaocluster/configuration/unseal',
    },
    {
      label: 'Recover from no leader',
      description: 'Switch here when sealing is fixed but the cluster still cannot elect or keep a leader.',
      docId: 'user-guide/openbaocluster/recovery/no-leader',
    },
    {
      label: 'Enter safe mode',
      description: 'Inspect and acknowledge break glass only after the seal path and workload health are stable.',
      docId: 'user-guide/openbaocluster/recovery/safe-mode',
    },
    {
      label: 'Run a restore',
      description: 'Use the restore workflow if the live cluster is no longer the safest path to service recovery.',
      docId: 'user-guide/openbaorestore/restore',
    },
  ]}
/>
