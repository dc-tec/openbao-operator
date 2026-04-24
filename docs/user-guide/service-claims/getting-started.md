---
title: Apply the first claim
description: Apply a same-cluster OpenBaoClusterClaim against a published service offering and verify the local workload and connection contract.
slug: /service-claims/get-started
hide_title: true
pageType: task
journey: get-started
---

<PageHeader
  title="Apply the first same-cluster service claim"
  lede="Use this quickstart after the operator install, tenant onboarding, and service catalog are already in place. The platform team publishes the catalog objects. Tenant users submit only the claim."
/>

<Checklist
  title="Prerequisites"
  items={[
    'install the operator with the service-claim surface enabled and admission policies active',
    'finish OpenBaoTenant onboarding for the target namespace in multi-tenant mode',
    'confirm the platform team already published the service offering and its backing catalog objects',
    'know the tenant name and service-offering name you are allowed to use',
  ]}
/>

<Callout type="note" title="Supported runtime shape">

The supported public path today is same-cluster materialization. The claim binds to a platform-owned catalog offering and the controller materializes a local `OpenBaoCluster` in the tenant namespace.

</Callout>

## Apply the claim

<CommandBlock
  language="yaml"
  label="configure"
  title="Create a claim that selects a stable service offering"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoClusterClaim
metadata:
  name: team-a-vault
  namespace: team-a-prod
spec:
  tenantRef:
    name: team-a-onboarding
  serviceOfferingRef:
    name: dev-internal`}
>
  `serviceOfferingRef` is the normal tenant-facing entry point. The claim binds through the stable offering alias and the platform pins the request to the current immutable service-profile revision behind that alias.
</CommandBlock>

<CommandBlock
  language="bash"
  label="apply"
  title="Apply the claim"
  code={`kubectl apply -f claim.yaml`}
/>

## Verify the claim and the materialized workload

<CommandBlock
  language="bash"
  label="inspect"
  title="Inspect the claim status"
  code={`kubectl get openbaoclusterclaim team-a-vault -n team-a-prod -o yaml`}
>
  Watch `status.phase`, `status.materialization.mode`, `status.materialization.localRef`, and `status.connection`. A healthy same-cluster claim reaches `phase: Ready` and records the local materialized cluster reference.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Inspect the materialized local cluster"
  code={`kubectl get openbaocluster -n team-a-prod
kubectl get pods -l openbao.org/cluster=team-a-vault -n team-a-prod`}
>
  The materialized cluster remains the workload boundary. Use it when you need deeper workload status, pods, or events behind the claim.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Inspect the published connection contract"
  code={`kubectl get secret team-a-vault-connection -n team-a-prod -o yaml
kubectl get openbaoclusterclaim team-a-vault -n team-a-prod -o jsonpath='{.status.connection.endpoint}{"\n"}'`}
>
  The connection Secret and endpoint are the tenant-facing output contract. For external exposure, the endpoint is published only after ingress or gateway integration is actually ready.
</CommandBlock>

## Know what the platform team owns

Tenant users create only the claim. The platform team owns the catalog objects behind it, including:

- `OpenBaoServiceOffering`
- `OpenBaoServiceProfile`
- `OpenBaoStorageProfile`, `OpenBaoUnsealProfile`, `OpenBaoRuntimeProfile`, `OpenBaoNetworkProfile`, `OpenBaoUpgradePolicy`, and `OpenBaoObservabilityProfile` when the selected service profile references them
- `OpenBaoExposureClass`
- `OpenBaoIngressPolicy` when ingress publication is used
- bootstrap and backup policy objects the service profile references

<Callout type="warning" title="Materialized claims are locked">

After the claim is materially bound, the spec is no longer a free-form edit surface. The admission model locks post-materialization service selection and protects claim-managed local clusters from direct mutation or deletion.

</Callout>

<NextActions
  title="Continue the claim workflow"
  items={[
    {
      label: 'Run day-2 workflows',
      description: 'Use explicit request objects for in-place upgrades, manual backups, and restores.',
      docId: 'user-guide/service-claims/day-2-workflows',
    },
    {
      label: 'Review the service catalog',
      description: 'Understand which catalog objects exist behind the claim and which of them remain mutable.',
      docId: 'user-guide/service-claims/service-catalog',
    },
    {
      label: 'Troubleshoot a claim',
      description: 'Route pending, failed, or not-yet-published claims to the right surface quickly.',
      docId: 'user-guide/service-claims/troubleshooting',
    },
  ]}
/>
