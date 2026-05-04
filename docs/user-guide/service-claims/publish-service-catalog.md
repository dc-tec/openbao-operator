---
title: Publish a minimum service catalog
description: Create the smallest platform-owned catalog that makes OpenBaoClusterClaim usable for an internal development service.
slug: /service-claims/publish-catalog
hide_title: true
pageType: task
journey: get-started
---

<PageHeader
  title="Publish a minimum service catalog"
  lede="Platform admins publish the catalog before tenant users create claims. Start with one internal offering, one immutable service profile, and the smallest policy objects that profile references."
/>

<Checklist
  title="Prerequisites"
  items={[
    'install the operator with the service-claim surface enabled',
    'run in multi-tenant mode when tenant namespaces are introduced through OpenBaoTenant',
    'understand the catalog object model',
    'confirm that the first service shape is supported by the catalog matrix',
  ]}
/>

<Callout type="note" title="Separate platform and tenant work">

The service catalog is platform-owned. Tenant users should normally create only
`OpenBaoClusterClaim` objects against an allowed `OpenBaoServiceOffering`.

</Callout>

## Choose the first catalog shape

<DecisionTable
  title="Use the minimum catalog for the first path"
  columns={['Catalog shape', 'Use it for', 'Required catalog surface']}
  rows={[
    {
      cells: [
        'Development internal',
        'Local evaluation or non-production namespaces where a small single-voter service is enough.',
        'Service offering, service profile, exposure class, bootstrap profile, backup-disabled profile.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Hardened backup',
        'Production-like same-cluster service with non-static unseal, scheduled backups, strict egress, and explicit helper images.',
        'Use the production catalog page. Add unseal, storage, runtime, network, observability, backup, and backup-target profiles.',
      ],
    },
    {
      cells: [
        'Full production catalog',
        'A reusable production offering with read replicas, external publication, ACME or external TLS, blue/green defaults, and bounded tenant parameters.',
        'Use the production catalog page. Add upgrade policy, read-replica runtime settings, exposure hostname policy, and ACME RWX cache storage if ACME is selected.',
      ],
    },
  ]}
/>

## Publish the minimum catalog

The minimum useful catalog has an immutable service profile and a mutable
offering alias. The service profile points at platform-owned policy objects. New
claims bind through the offering; existing claims keep the immutable revision
they already applied.

<CommandBlock
  language="yaml"
  label="configure"
  title="Publish a development internal catalog"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoExposureClass
metadata:
  name: dev-internal-v1
spec:
  publishMode: ClusterInternal
  hostnamePolicy:
    mode: Generated
    domainSuffix: svc.cluster.local
  tlsPolicy:
    mode: OperatorManaged
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoBootstrapProfile
metadata:
  name: bootstrap-basic-v1
spec:
  operatorLifecycleAuth:
    mode: JWT
    jwt:
      audience: openbao-operator
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoBackupProfile
metadata:
  name: backup-disabled-v1
spec: {}
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoServiceProfile
metadata:
  name: dev-internal-v1
spec:
  cluster:
    version: "2.5.0"
    voters: 1
    securityProfile: Development
  storage:
    primarySize: 10Gi
  bootstrap:
    mode: SelfInit
    profileRef:
      name: bootstrap-basic-v1
  exposure:
    classRef:
      name: dev-internal-v1
  backup:
    profileRef:
      name: backup-disabled-v1
  lifecycle:
    upgradeStrategy: RollingUpdate
    preUpgradeSnapshot: true
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoServiceOffering
metadata:
  name: dev-internal
spec:
  currentRevisionRef:
    name: dev-internal-v1`}
>
  Keep the offering name stable and version the service profile name. Publish a new service-profile revision when the service contract changes.
</CommandBlock>

<CommandBlock
  language="bash"
  label="apply"
  title="Apply the checked-in minimum catalog sample"
  code={`kubectl apply -f config/samples/claims/dev-internal-catalog.yaml`}
>
  The sample matches the YAML above and is the fastest way to test the minimum catalog path locally.
</CommandBlock>

## Verify the catalog before handing it to tenants

<CommandBlock
  language="bash"
  label="verify"
  title="Inspect catalog objects"
  code={`kubectl get openbaoserviceoffering
kubectl get openbaoserviceprofile
kubectl get openbaoexposureclass
kubectl get openbaobootstrapprofile
kubectl get openbaobackupprofile`}
>
  A tenant-facing offering is ready only when the offering, its immutable service profile, and every referenced catalog object exist.
</CommandBlock>

<NextActions
  title="Continue the service-claim path"
  items={[
    {
      label: 'Request a service',
      description: 'Create the tenant-facing claim once the service offering is published.',
      docId: 'user-guide/service-claims/getting-started',
    },
    {
      label: 'Publish a production catalog',
      description: 'Add hardened implementation profiles, backups, edge publication, and bounded tenant parameters.',
      docId: 'user-guide/service-claims/publish-production-catalog',
    },
    {
      label: 'Review catalog support',
      description: 'Check which direct OpenBaoCluster fields are represented by the catalog and which remain intentionally unsupported.',
      docId: 'user-guide/service-claims/support-matrix',
    },
    {
      label: 'Plan claim exposure',
      description: 'Tune internal, ingress, gateway, TLS, ACME, and bounded hostname behavior through exposure policy.',
      docId: 'user-guide/service-claims/exposure',
    },
  ]}
/>
