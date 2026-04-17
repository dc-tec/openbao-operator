---
title: Recover from No Leader
description: Diagnose whether quorum loss comes from pod health, network isolation, stale peers, or a last-resort manual Raft recovery path.
slug: /recover/no-leader
hide_title: true
pageType: runbook
journey: operate
---

<PageHeader
  title="Diagnose no-leader conditions before manual Raft recovery"
  lede="A no-leader incident can come from crash-looping pods, blocked cluster traffic, or stale peers that still count toward quorum. Use this runbook to narrow the failure mode and decide whether a manual quorum-recovery path is actually required."
/>

<Checklist
    title="Use this runbook when"
    items={[
      'the cluster cannot elect or keep a leader',
      'Raft commands time out or report no leader elected',
      'pods are running but the cluster still cannot form quorum',
      'you need to decide whether a stale peer, network break, or manual recovery path is required',
    ]}
  />


<DecisionTable
  title="Match the failure mode"
  columns={['Signal', 'Start with', 'Why']}
  rows={[
    {
      cells: [
        'Pods are crash-looping or never become ready.',
        'Fix pod health, storage, or config first.',
        'Quorum cannot recover while one or more members are not healthy enough to participate in Raft.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        '`raft list-peers` hangs or returns transport errors.',
        'Check network reachability and DNS on the cluster port.',
        'A healthy Raft set still fails if members cannot talk to each other on the internal cluster address.',
      ],
    },
    {
      cells: [
        'A failed or stale member still appears in the peer list.',
        'Remove the dead peer from a healthy node.',
        'The cluster may be counting a non-existent member toward quorum.',
      ],
    },
    {
      cells: [
        'No node can form quorum and there is no healthy leader to remove peers from.',
        'Use manual quorum recovery with `peers.json` as a last resort.',
        'This is destructive. Use it only when automatic recovery is no longer possible.',
      ],
    },
  ]}
/>

<DiagramFrame
  title="No-leader decision flow"
  caption="Check pod health first, then verify the cluster network, then decide whether this is a stale-peer cleanup or a manual quorum-recovery event."
  code={`flowchart TD
    Start["No leader"] --> Pods{"Pods healthy?"}
    Pods -- "No" --> FixPods["Repair crashes, PVCs, or config"]
    Pods -- "Yes" --> Net{"Cluster port reachable?"}
    Net -- "No" --> FixNet["Repair DNS, NetworkPolicy, or routing"]
    Net -- "Yes" --> Peers{"Stale peer in raft list?"}
    Peers -- "Yes" --> RemovePeer["Remove dead peer from healthy node"]
    Peers -- "No" --> Quorum{"Any node can still form quorum?"}
    Quorum -- "Yes" --> Inspect["Inspect logs and conditions again"]
    Quorum -- "No" --> Manual["Manual quorum recovery with peers.json"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Start,Pods,Net,Peers,Quorum read;
    class FixPods,FixNet,Inspect process;
    class RemovePeer,Manual write;`}
/>

## Inspect pod health and the Raft view

<CommandBlock
  language="bash"
  label="inspect"
  title="Capture pod and peer state"
  code={`kubectl get pods -n <namespace> -l openbao.org/cluster=<name> -o wide
kubectl get openbaocluster <name> -n <namespace> \\
  -o jsonpath='{range .status.conditions[*]}{.type}={.status} {.reason}{"\\n"}{end}'
kubectl exec -n <namespace> -it <pod-name> -- bao operator raft list-peers`}
>
  Run the Raft peer command from each healthy Pod. Different peer views between nodes usually point to a transport or membership problem rather than a generic application issue.
</CommandBlock>

If Pods are `CrashLoopBackOff`, resolve the pod-level failure first. Quorum recovery does not help when the underlying member still cannot boot, mount storage, or load configuration.

## Verify the cluster network path

<CommandBlock
  language="bash"
  label="inspect"
  title="Check the Raft transport path"
  code={`kubectl exec -n <namespace> -it <pod-a> -- nc -zv <pod-b>.<headless-service> 8201
kubectl exec -n <namespace> -it <pod-a> -- nslookup <pod-b>.<headless-service>`}
>
  Use the actual cluster port if you changed it from the default. Failed DNS or blocked port `8201` is enough to make a healthy cluster look like a quorum failure.
</CommandBlock>

Check these first when the transport path is broken:

- `NetworkPolicy` rules that allow cluster-to-cluster traffic
- service and headless-service DNS resolution
- sidecar or mesh policies that may block direct Pod-to-Pod communication

## Remove a stale peer when quorum is almost healthy

Use this path only when one healthy node can still answer Raft commands and the peer list clearly shows a dead member that should no longer count toward quorum.

<CommandBlock
  language="bash"
  label="apply"
  title="Remove a dead peer from the Raft set"
  code={`kubectl exec -n <namespace> -it <healthy-pod> -- \\
  bao operator raft remove-peer -id "<dead-peer-id>"`}
/>

After removing the peer, re-run `bao operator raft list-peers` from the same node and watch the `OpenBaoCluster` conditions until a leader is present again.

## Use manual quorum recovery only as a last resort

<Callout type="danger" title="Manual quorum recovery can discard newer state">

Creating `peers.json` forces one survivor to bootstrap a new one-node Raft cluster. If you pick a node that was behind, you can lose data that only existed on another member. Use this path only when normal quorum recovery is no longer possible.

</Callout>

Before you start:

- stop the operator so it does not race your manual changes
- choose the Pod with the most recent and trustworthy data
- record which Pods you plan to restart and how you will rejoin them afterward

<CommandBlock
  language="bash"
  label="apply"
  title="Stop operator reconciliation before manual recovery"
  code={`kubectl scale deploy <controller-deployment> -n <operator-namespace> --replicas=0`}
/>

<CommandBlock
  language="json"
  label="configure"
  title="Create a one-node peers.json file"
  code={`[
  {
    "id": "<survivor-pod-name>",
    "address": "<survivor-pod-name>.<headless-service>:8201",
    "non_voter": false
  }
]`}
>
  Save this file locally as `peers.json`, replacing the Pod name and address with the survivor you selected.
</CommandBlock>

<CommandBlock
  language="bash"
  label="apply"
  title="Copy peers.json to the survivor and restart it"
  code={`kubectl cp peers.json <namespace>/<survivor-pod-name>:/bao/data/raft/peers.json
kubectl delete pod -n <namespace> <survivor-pod-name>`}
/>

When the survivor returns:

1. verify it now reports a leader-capable state
2. unseal it if required
3. delete the remaining Pods so they rejoin against the new leader
4. restart the operator deployment

<CommandBlock
  language="bash"
  label="verify"
  title="Resume normal operations after manual recovery"
  code={`kubectl exec -n <namespace> -it <survivor-pod-name> -- bao operator raft list-peers
kubectl delete pod -n <namespace> <other-pod-1> <other-pod-2>
kubectl scale deploy <controller-deployment> -n <operator-namespace> --replicas=1`}
/>

## Close out the incident deliberately

If the operator entered break glass during the failure, inspect the break-glass status and only acknowledge it after quorum, sealing, and workload health are all stable again.

See <SiteLink docId="user-guide/openbaocluster/recovery/safe-mode">Enter Safe Mode</SiteLink> for the acknowledgment flow.

<NextActions
  title="Continue with the right recovery path"
  items={[
    {
      label: 'Enter safe mode',
      description: 'Inspect the break-glass nonce and resume automation only after the cluster is stable again.',
      docId: 'user-guide/openbaocluster/recovery/safe-mode',
    },
    {
      label: 'Recover a sealed cluster',
      description: 'Switch to the seal-focused runbook when Pods are up but trust or unseal dependencies still block service.',
      docId: 'user-guide/openbaocluster/recovery/sealed-cluster',
    },
    {
      label: 'Run a restore',
      description: 'Use the restore workflow if live-cluster repair is no longer the safest option.',
      docId: 'user-guide/openbaorestore/restore',
    },
  ]}
/>
