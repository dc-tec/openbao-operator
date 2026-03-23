---
title: Run Planned Maintenance
description: Drain nodes, scale the cluster, enable maintenance mode, and restart Pods without guessing how quorum and admission policy interact.
slug: /operate/maintenance
hide_title: true
pageType: task
journey: operate
---

<PageHero
  eyebrow="Operate / Cluster controls"
  title="Use cluster controls deliberately when you need to change the platform underneath OpenBao."
  lede="Planned maintenance is where Kubernetes disruption rules, Raft quorum, and admission-policy guardrails all meet. Use this page to prepare drains and restarts, scale safely, and confirm the cluster is still healthy before you hand it back to normal operations."
  actions={[
    {label: 'Pause reconciliation', docId: 'user-guide/openbaocluster/operations/pausing', variant: 'primary'},
    {label: 'Open recovery and restore', docId: 'user-guide/openbaocluster/recovery/index', variant: 'secondary'},
  ]}
>
  <Checklist
    title="Use this page when you need to"
    items={[
      'drain nodes that currently host OpenBao Pods',
      'scale the cluster up or down deliberately',
      'enable maintenance mode for controlled restarts under strict admission policy',
      'restart or repair the workload without turning a planned change into an incident',
    ]}
  />
</PageHero>

<DecisionTable
  title="Choose the right control"
  columns={['Control', 'Use it when', 'Operator behavior', 'Watch for']}
  rows={[
    {
      cells: [
        'Node drain with PDB protection',
        'You are changing the Kubernetes substrate and want the normal workload to keep serving.',
        'The PodDisruptionBudget blocks voluntary disruption that would take too many voters offline at once.',
        'The PDB only protects against voluntary evictions, not hard node failures.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Replica scaling',
        'You need more capacity, stronger fault tolerance, or a deliberate reduction after a change.',
        'The operator grows or shrinks the StatefulSet and manages peer membership as the replica count changes.',
        'Do not treat scale-down as a harmless cost-saving action on a production Raft cluster.',
      ],
    },
    {
      cells: [
        'Maintenance mode',
        'Admission policy requires the `openbao.org/maintenance=true` signal before restarts or controlled deletes.',
        'The operator annotates managed resources so maintenance-only actions are allowed under the configured break-glass groups.',
        'This is not a generic bypass for random edits. It is a controlled operational mode.',
      ],
    },
    {
      cells: [
        'Pause reconciliation',
        'You need a short-lived window where the operator stops mutating the cluster while you inspect or repair it.',
        'The operator stops normal reconciliation until you resume it.',
        'Pausing is not the same thing as recovery and is not enough for safe-mode incidents.',
      ],
    },
  ]}
/>

## Drain nodes without breaking quorum

For clusters with three or more replicas, the operator creates a `PodDisruptionBudget` with `maxUnavailable: 1`. That is the main guardrail that keeps a normal node drain from evicting too many Pods at once.

<DecisionTable
  kind="reference"
  title="Pod disruption behavior by replica count"
  columns={['Replicas', 'PDB created', 'What it means']}
  rows={[
    {
      cells: [
        '1',
        'No',
        'There is no redundancy. Any disruption takes the service down.',
      ],
    },
    {
      cells: [
        '2',
        'No',
        'A two-node Raft cluster cannot tolerate one unavailable voter cleanly enough for a safe `maxUnavailable: 1` policy.',
      ],
    },
    {
      cells: [
        '3',
        'Yes',
        'The PDB keeps two voters available while one Pod is evicted.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        '5',
        'Yes',
        'The operator still uses a conservative one-at-a-time disruption model.',
      ],
    },
  ]}
/>

<CommandBlock
  language="bash"
  label="verify"
  title="Check the disruption budget before a drain"
  code={`kubectl get pdb -n <namespace>
kubectl drain <node-name> --ignore-daemonsets --delete-emptydir-data`}
>
  If more than one OpenBao Pod is concentrated on the same node, the drain may take longer because Kubernetes has to evict the Pods sequentially.
</CommandBlock>

<Callout type="note" title="The PDB covers only voluntary disruption">

Node drains, autoscaler evictions, and direct eviction API calls are guarded. Node crashes, OOM kills, or kernel failures are not. Those rely on normal Raft quorum behavior instead of disruption-budget enforcement.

</Callout>

## Scale the cluster deliberately

Use scaling as an intentional operational change, not a quick patch to quiet a temporary issue.

<Tabs groupId="maintenance-scaling">

<TabItem value="scale-up" label="Scale up">

<CommandBlock
  language="yaml"
  label="configure"
  title="Increase the replica count"
  code={`spec:
  replicas: 5`}
>
  The operator creates the new Pods, waits for them to join the Raft cluster, and updates the PDB to match the new size.
</CommandBlock>

</TabItem>

<TabItem value="scale-down" label="Scale down">

<CommandBlock
  language="yaml"
  label="configure"
  title="Reduce the replica count"
  code={`spec:
  replicas: 3`}
>
  The operator removes voters from the highest ordinal first, waits for the Raft configuration to converge, and only then deletes the excess Pods.
</CommandBlock>

<Callout type="warning" title="Do not scale below three replicas in production">

Reducing a production cluster below three replicas removes the redundancy that makes ordinary node and Pod failures survivable.

</Callout>

</TabItem>

</Tabs>

## Use maintenance mode for controlled restarts

Enable maintenance mode when your admission policies require a deliberate maintenance signal before managed Pods or the StatefulSet can be restarted, deleted, or otherwise touched during planned work.

<CommandBlock
  language="yaml"
  label="configure"
  title="Enable maintenance mode"
  code={`spec:
  maintenance:
    enabled: true`}
>
  In this mode, the operator annotates managed Pods and the StatefulSet with `openbao.org/maintenance=true`. By default, maintenance-only bypass is limited to callers in the Kubernetes group `system:masters` unless you changed the configured break-glass groups at install time.
</CommandBlock>

This mode is also required for some day 2 changes that need a controlled restart path, such as finishing filesystem expansion after increasing `spec.storage.size`.

## Trigger a rolling restart

Use `restartAt` when you need the workload to roll because an external dependency changed, such as a certificate chain, secret material, or another input that should force a controlled refresh.

<CommandBlock
  language="yaml"
  label="configure"
  title="Request a rolling restart"
  code={`spec:
  maintenance:
    restartAt: "2026-01-19T00:00:00Z"`}
/>

When a leader Pod must be restarted or evicted, the operator handles graceful step-down automatically before termination so the cluster can elect a new leader cleanly.

## Verify the cluster before and after the window

<CommandBlock
  language="bash"
  label="verify"
  title="Inspect health before and after maintenance"
  code={`kubectl get openbaocluster <name> -n <namespace> -o jsonpath='{.status.phase}{"\\n"}'
kubectl get pods -n <namespace> -l openbao.org/cluster=<name>
kubectl exec -n <namespace> -it <pod-name> -- bao operator raft list-peers`}
>
  The important end state is a clean phase, Ready Pods, and a Raft peer set that matches the intended topology after the maintenance action finishes.
</CommandBlock>

## External references

- [Raft integrated storage](https://openbao.org/docs/internals/integrated-storage)

<NextActions
  title="Move to the next control"
  items={[
    {
      label: 'Pause reconciliation',
      description: 'Use the lighter-weight control when you need the operator to stop mutating the cluster during a short repair window.',
      docId: 'user-guide/openbaocluster/operations/pausing',
    },
    {
      label: 'Decommission a cluster',
      description: 'Choose the right teardown policy before you remove a cluster and its storage.',
      docId: 'user-guide/openbaocluster/operations/deletion',
    },
    {
      label: 'Open recovery and restore',
      description: 'Escalate into the incident paths when planned maintenance turns into a leader, seal, or rollback problem.',
      docId: 'user-guide/openbaocluster/recovery/index',
    },
  ]}
/>
