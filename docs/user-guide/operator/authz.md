---
title: Operator Authorization
description: Understand which policies belong to controller, backup, restore, and upgrade work so destructive capabilities stay scoped to the right identities.
slug: /get-started/operator-authorization
hide_title: true
pageType: concept
journey: get-started
---

<PageHeader
  title="Keep each operator capability on its own policy surface."
  lede="Authentication answers who a workload is. Authorization answers what that workload can do. OpenBao Operator stays safer when controller, backup, restore, and upgrade work authenticate separately and only receive the policies each path actually needs."
/>



<DiagramFrame
  title="Policies stay attached to job-specific identities"
  caption="Each operator path maps to its own JWT role and policy set. The controller is not the universal credential for all day 2 work."
  code={`graph LR
    subgraph K8s["Kubernetes identities"]
      Controller["Controller SA"]
      Backup["Backup Job SA"]
      Restore["Restore Job SA"]
      Upgrade["Upgrade Job SA"]
    end

    subgraph Bao["OpenBao auth and policy"]
      RoleController["Role: openbao-operator"]
      RoleBackup["Role: openbao-operator-backup"]
      RoleRestore["Role: openbao-operator-restore"]
      RoleUpgrade["Role: openbao-operator-upgrade"]

      PolicyController["Policy: controller maintenance"]
      PolicyBackup["Policy: snapshot read"]
      PolicyRestore["Policy: snapshot-force"]
      PolicyUpgrade["Policy: rolling or blue-green upgrade"]
    end

    Controller --> RoleController --> PolicyController
    Backup --> RoleBackup --> PolicyBackup
    Restore --> RoleRestore --> PolicyRestore
    Upgrade --> RoleUpgrade --> PolicyUpgrade

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;
    classDef caution fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;

    class Controller,Backup,Restore,Upgrade read;
    class RoleController,RoleBackup,RoleRestore,RoleUpgrade write;
    class PolicyController,PolicyBackup,PolicyUpgrade write;
    class PolicyRestore caution;`}
/>

<DecisionTable
  title="Keep policies separated by lifecycle capability"
  columns={['Policy surface', 'Used by', 'Typical capabilities', 'Why it stays separate']}
  rows={[
    {
      cells: [
        'Controller maintenance',
        'The main controller Deployment',
        '`sys/health`, `sys/step-down`, and autopilot configuration reads or updates',
        'This path should stay available for routine reconciliation and maintenance without inheriting destructive restore powers.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Backup',
        'The generated backup Job',
        '`sys/storage/raft/snapshot` read access',
        'Snapshot reads are narrower than normal controller maintenance and should be easy to reason about independently.',
      ],
    },
    {
      cells: [
        'Restore',
        'The generated restore Job',
        '`sys/storage/raft/snapshot-force` update access',
        'Restore can replace the full cluster state and should only exist on the specific workload that performs restore.',
      ],
      emphasis: 'caution',
    },
    {
      cells: [
        'Upgrade',
        'The generated upgrade Job',
        'Step-down, autopilot state, snapshot read, and optional peer-management operations for blue-green flows',
        'Upgrade paths often need time-bounded orchestration permissions that should not widen steady-state controller access.',
      ],
    },
  ]}
/>

<Callout type="danger" title="Treat restore as a destructive role">

The restore capability can replace data, policies, and keys across the cluster.
Do not bind the restore policy to the controller or to a broad multi-purpose ServiceAccount just because it is convenient during setup.

</Callout>

## Default policy surfaces

<Tabs groupId="operator-policy-surfaces">

<TabItem value="controller" label="Controller">

<CommandBlock
  language="hcl"
  label="configure"
  title="Controller maintenance policy"
  code={`path "sys/health" {
  capabilities = ["read"]
}

path "sys/step-down" {
  capabilities = ["sudo", "update"]
}

path "sys/storage/raft/autopilot/configuration" {
  capabilities = ["read", "update"]
}`}
>
  This is the steady-state controller scope. It should not expand to cover backup, restore, or blue-green peer management unless you are intentionally breaking the model.
</CommandBlock>

</TabItem>

<TabItem value="backup" label="Backup">

<CommandBlock
  language="hcl"
  label="configure"
  title="Backup policy"
  code={`path "sys/storage/raft/snapshot" {
  capabilities = ["read"]
}`}
>
  Backup only needs snapshot streaming. Storage-provider credentials are a separate boundary outside this OpenBao policy.
</CommandBlock>

</TabItem>

<TabItem value="restore" label="Restore">

<CommandBlock
  language="hcl"
  label="configure"
  title="Restore policy"
  code={`path "sys/storage/raft/snapshot-force" {
  capabilities = ["update"]
}`}
>
  Keep this policy tightly bound to the generated restore Job identity and only for the period where restore is actually needed.
</CommandBlock>

</TabItem>

<TabItem value="upgrade" label="Upgrade">

<CommandBlock
  language="hcl"
  label="configure"
  title="Rolling and blue-green upgrade policy surfaces"
  code={`# Rolling upgrade baseline
path "sys/health" {
  capabilities = ["read"]
}

path "sys/step-down" {
  capabilities = ["sudo", "update"]
}

path "sys/storage/raft/autopilot/state" {
  capabilities = ["read"]
}

# Blue-green adds peer-management paths
path "sys/storage/raft/join" {
  capabilities = ["update"]
}

path "sys/storage/raft/configuration" {
  capabilities = ["read", "update"]
}

path "sys/storage/raft/remove-peer" {
  capabilities = ["update"]
}

path "sys/storage/raft/promote" {
  capabilities = ["update"]
}

path "sys/storage/raft/demote" {
  capabilities = ["update"]
}`}
>
  Rolling upgrades need less authority than blue-green cutovers. Keep those strategies separate in your head when you review the required policy surface.
</CommandBlock>

</TabItem>

</Tabs>

<DecisionTable
  kind="reference"
  title="Common authorization failures"
  columns={['Symptom', 'Likely boundary', 'Check first']}
  rows={[
    {
      cells: [
        'JWT login succeeds but the request returns `permission denied`',
        'The workload authenticated correctly but the policy is missing the needed path capability',
        'Which job or controller path is making the request, then the matching policy surface',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Backup works but restore fails',
        'The restore Job identity or restore policy is missing or misbound',
        'Restore ServiceAccount, restore role binding, and `snapshot-force` policy',
      ],
    },
    {
      cells: [
        'Rolling upgrade works but blue-green cutover stalls',
        'Peer-management permissions were not added for the upgrade strategy in use',
        'Upgrade strategy and the corresponding upgrade policy paths',
      ],
    },
    {
      cells: [
        'Controller can do too much',
        'A shortcut merged job-specific capabilities into the controller role',
        'Manual auth configuration drift from the intended separation model',
      ],
    },
  ]}
/>

<NextActions
  title="Go deeper"
  items={[
    {
      label: 'Operator authentication',
      description: 'Return to the JWT audience and bound-subject model when auth fails before policy even matters.',
      docId: 'user-guide/operator/authn',
    },
    {
      label: 'Backup operations',
      description: 'See how the backup Job uses its own auth and storage credentials during normal operation.',
      docId: 'user-guide/openbaocluster/operations/backups',
    },
    {
      label: 'Restore manager architecture',
      description: 'Review why restore stays isolated from the controller and how the operator drives it.',
      docId: 'architecture/restore-manager',
    },
  ]}
/>

## Official OpenBao background

- [Policy concepts](https://openbao.org/docs/concepts/policies/)
- [Policy command reference](https://openbao.org/docs/commands/policy/)
- [Token concepts](https://openbao.org/docs/concepts/tokens/)
