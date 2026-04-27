---
title: Publish a service catalog
description: Create the platform-owned catalog objects that make OpenBaoClusterClaim usable for tenant service requests.
slug: /service-claims/publish-catalog
hide_title: true
pageType: task
journey: get-started
---

<PageHeader
  title="Publish a service catalog"
  lede="Platform admins publish the catalog before tenant users create claims. Start with one offering, one immutable service profile, and the policy objects that profile references."
/>

<Checklist
  title="Prerequisites"
  items={[
    'install the operator with the service-claim surface enabled',
    'run in multi-tenant mode when tenant namespaces are introduced through OpenBaoTenant',
    'choose the first service shape the platform is willing to support',
    'decide whether the first catalog is development-only or production hardened',
  ]}
/>

<Callout type="note" title="Separate platform and tenant work">

The service catalog is platform-owned. Tenant users should normally create only
`OpenBaoClusterClaim` objects against an allowed `OpenBaoServiceOffering`.

</Callout>

## Choose the first catalog shape

<DecisionTable
  title="Start with one service shape"
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
        'Development internal surface plus unseal, storage, runtime, network, observability, backup, and backup-target profiles.',
      ],
    },
    {
      cells: [
        'Full production catalog',
        'A reusable production offering with read replicas, external publication, ACME or external TLS, blue/green defaults, and bounded tenant parameters.',
        'Hardened backup surface plus upgrade policy, read-replica profile settings, exposure hostname policy, and ACME RWX cache storage if ACME is selected.',
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

## Add production implementation profiles

Production catalogs should keep implementation details out of the tenant-facing
claim. Add profiles for environment-specific storage, runtime, network,
observability, unseal, and upgrade policy.

<CommandBlock
  language="yaml"
  label="configure"
  title="Add reusable platform implementation profiles"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoStorageProfile
metadata:
  name: production-storage-v1
spec:
  primary:
    storageClassName: fast-ssd
  readReplica:
    usePrimaryStorageClass: true
  acmeCache:
    mode: ManagedPVC
    size: 1Gi
    storageClassName: nfs-rwx
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoRuntimeProfile
metadata:
  name: production-runtime-v1
spec:
  serviceAccount:
    annotations:
      example.com/workload-identity: openbao
  helperImages:
    init: ghcr.io/example/openbao-init:2.5
    backup: ghcr.io/example/openbao-backup:2.5
    restore: ghcr.io/example/openbao-backup:2.5
    upgrade: ghcr.io/example/openbao-upgrade:2.5
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoNetworkProfile
metadata:
  name: production-network-v1
spec:
  dnsNamespace: kube-system
  egressRules: []
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoObservabilityProfile
metadata:
  name: production-observability-v1
spec:
  observability:
    metrics:
      enabled: true
      serviceMonitor:
        enabled: true
        interval: 30s
        scrapeTimeout: 10s
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoUnsealProfile
metadata:
  name: production-transit-v1
spec:
  mode: Transit
  transit:
    address: https://transit-bao.openbao-infra.svc:8200
    keyName: openbao-unseal
    mountPath: transit
  credentialsSecretRef:
    name: transit-unseal-token
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoUpgradePolicy
metadata:
  name: production-bluegreen-v1
spec:
  blueGreen:
    autoPromote: false
    minSyncDuration: 30s
    maxJobFailures: 1
    autoRollback:
      enabled: true
      onJobFailure: true
      onValidationFailure: true`}
/>

Reference those profiles from the production service profile instead of adding
raw workload knobs to claims.

<CommandBlock
  language="yaml"
  label="configure"
  title="Publish a production service profile"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoServiceProfile
metadata:
  name: hardened-edge-v1
spec:
  cluster:
    version: "2.5.0"
    voters: 3
    readReplicas: 1
    securityProfile: Hardened
  storage:
    primarySize: 20Gi
    readReplicaSize: 10Gi
    profileRef:
      name: production-storage-v1
  bootstrap:
    mode: SelfInit
    profileRef:
      name: bootstrap-basic-v1
  exposure:
    classRef:
      name: gateway-public-v1
  unseal:
    profileRef:
      name: production-transit-v1
  runtime:
    profileRef:
      name: production-runtime-v1
  observability:
    profileRef:
      name: production-observability-v1
  network:
    profileRef:
      name: production-network-v1
  backup:
    profileRef:
      name: object-storage-scheduled-v1
  lifecycle:
    policyRef:
      name: production-bluegreen-v1
    upgradeStrategy: BlueGreen
    preUpgradeSnapshot: true
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoServiceOffering
metadata:
  name: hardened-edge
spec:
  currentRevisionRef:
    name: hardened-edge-v1`}
/>

## Allow only bounded tenant parameters

Tenant-provided values stay inside `spec.serviceParameters` and only work when
the selected catalog object permits them. Do not add tenant-owned raw storage,
runtime, unseal, or network fields to avoid publishing another direct
`OpenBaoCluster` API.

<CommandBlock
  language="yaml"
  label="configure"
  title="Allow bounded hostnames in the exposure class"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoExposureClass
metadata:
  name: gateway-public-v1
spec:
  publishMode: Gateway
  hostnamePolicy:
    mode: Generated
    domainSuffix: apps.example.com
    claim:
      enabled: true
      allowedSuffixes:
        - apps.example.com
  tlsPolicy:
    mode: ACME
    acme:
      directoryURL: https://acme-v02.api.letsencrypt.org/directory
      email: platform@example.com
      domains:
        - "*.apps.example.com"`}
>
  Claims may request a hostname only when the exposure class enables it and the requested hostname matches the allowed suffix policy.
</CommandBlock>

<CommandBlock
  language="yaml"
  label="configure"
  title="Tenant claim with bounded parameters"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoClusterClaim
metadata:
  name: team-a-vault
  namespace: team-a-prod
spec:
  tenantRef:
    name: team-a-onboarding
  serviceOfferingRef:
    name: hardened-edge
  serviceParameters:
    exposure:
      hostname: team-a-vault.apps.example.com
    backup:
      partition: team-a`}
/>

## Verify the catalog before handing it to tenants

<CommandBlock
  language="bash"
  label="verify"
  title="Inspect catalog objects"
  code={`kubectl get openbaoserviceoffering
kubectl get openbaoserviceprofile
kubectl get openbaoexposureclass
kubectl get openbaobootstrapprofile
kubectl get openbaostorageprofile
kubectl get openbaoruntimeprofile
kubectl get openbaonetworkprofile
kubectl get openbaoobservabilityprofile
kubectl get openbaounsealprofile
kubectl get openbaoupgradepolicy
kubectl get openbaobackupprofile`}
>
  A tenant-facing offering is ready only when the offering, its immutable service profile, and every referenced catalog object exist.
</CommandBlock>

<NextActions
  title="Continue the service-claim path"
  items={[
    {
      label: 'Apply the first claim',
      description: 'Create the tenant-facing claim once the service offering is published.',
      docId: 'user-guide/service-claims/getting-started',
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
