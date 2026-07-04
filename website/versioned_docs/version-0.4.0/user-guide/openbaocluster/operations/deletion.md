---
title: Decommission a Cluster
description: Choose the deletion policy for a cluster teardown and verify what happens to PVCs, secrets, and backups.
slug: /operate/decommission
hide_title: true
pageType: task
journey: operate
---

<PageHeader
  title="Decommission a cluster"
  lede="Deleting an `OpenBaoCluster` removes the control plane, but the deletion policy determines whether PVC-backed data stays behind, whether critical secrets are preserved, and what still requires manual cleanup after the control plane is gone."
/>

<Checklist
    title="Use this page to"
    items={[
      'tear down a dev, staging, or production cluster intentionally',
      'change the deletion policy from the default retain behavior',
      'remove PVC-backed data as part of a deliberate teardown',
      'confirm whether snapshot backups still need manual cleanup afterward',
    ]}
  />


<DecisionTable
  title="Choose the deletion policy"
  columns={['Policy', 'Use it when', 'What gets deleted', 'Watch for']}
  rows={[
    {
      cells: [
        'Retain',
        'You want the safest default, especially for production clusters or any teardown where data may need to be recovered later.',
        'Compute resources are removed, but PVCs and the key secrets needed to recover the data remain.',
        'You must clean up retained data and secrets manually if the teardown is truly final.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'DeletePVCs',
        'You are tearing down an ephemeral or disposable environment and want the operator to remove PVC-backed data too.',
        'Compute resources, secrets, and PVCs are deleted.',
        'This is permanent data loss for the local cluster storage path.',
      ],
    },
    {
      cells: [
        'DeleteAll',
        'You want the most aggressive in-cluster cleanup the current implementation supports.',
        'Compute resources and PVCs are deleted, but external object-storage backups are still left behind.',
        'The API accepts the value, but external backup deletion is not implemented yet.',
      ],
    },
  ]}
/>

## Understand what is retained

The default `Retain` behavior preserves the things you need if the deletion turns out not to be final:

- PVC-backed data
- the unseal key Secret
- the root token Secret
- any external backups already stored in object storage

<Callout type="note" title="Why the unseal key is retained">

The unseal key material is what makes the retained PVC data usable later. If the operator let Kubernetes garbage-collect that Secret automatically, you could keep the encrypted data and still lose the ability to recover it.

</Callout>

## Configure the teardown policy

<Tabs groupId="deletion-policy">

<TabItem value="retain" label="Retain (Default)">

<CommandBlock
  language="yaml"
  label="configure"
  title="Keep data and critical secrets after cluster deletion"
  code={`spec:
  deletionPolicy: Retain`}
/>

</TabItem>

<TabItem value="delete-pvcs" label="DeletePVCs">

<CommandBlock
  language="yaml"
  label="configure"
  title="Delete PVC-backed data during teardown"
  code={`spec:
  deletionPolicy: DeletePVCs`}
/>

<Callout type="warning" title="This removes the local storage path">

Once the PVCs are deleted, the underlying volume data is gone unless you have an external snapshot or storage-level recovery path outside the operator.

</Callout>

The operator only deletes PVCs that carry OpenBao ownership proof, either through the `OpenBaoCluster` controller owner reference or the operator-written `openbao.org/owner-uid` annotation. Label-matched PVCs without that proof are left behind for manual review instead of being adopted or deleted by name.

</TabItem>

<TabItem value="delete-all" label="DeleteAll">

<CommandBlock
  language="yaml"
  label="configure"
  title="Request the most aggressive supported cleanup"
  code={`spec:
  deletionPolicy: DeleteAll`}
/>

<Callout type="warning" title="External backup deletion is still manual">

`DeleteAll` currently removes PVC-backed data but does not delete snapshot objects already written to S3, GCS, or Azure Blob Storage.

</Callout>

</TabItem>

</Tabs>

## Delete the cluster

<CommandBlock
  language="bash"
  label="apply"
  title="Delete the OpenBaoCluster"
  code={`kubectl delete openbaocluster <name> -n <namespace>`}
/>

If the cluster still serves production traffic, confirm your cutover, backup, and recovery assumptions before you continue.

## Verify the cleanup result

<CommandBlock
  language="bash"
  label="verify"
  title="Check for retained or remaining resources"
  code={`kubectl get pvc -n <namespace> -l openbao.org/cluster=<name>
kubectl get secret -n <namespace> -l openbao.org/cluster=<name>
kubectl get jobs -n <namespace> -l openbao.org/cluster=<name>`}
>
  Under `Retain`, leftover PVCs and critical secrets are expected. Under `DeletePVCs` or `DeleteAll`, the PVC-backed path should be gone, but external backups still need their own manual cleanup decision.
</CommandBlock>

<NextActions
  title="Continue the teardown or recovery path"
  items={[
    {
      label: 'Open restore operations',
      description: 'Use the restore workflow if the goal changed from teardown to rebuilding from a snapshot.',
      docId: 'user-guide/openbaorestore/restore',
    },
    {
      label: 'Review known limitations',
      description: 'Confirm the current pre-GA limitations around external backup deletion and other lifecycle edges.',
      docId: 'reference/known-limitations',
    },
    {
      label: 'Run planned maintenance',
      description: 'Go back to the maintenance controls if the cluster is not actually ready for final teardown.',
      docId: 'user-guide/openbaocluster/operations/maintenance',
    },
  ]}
/>
