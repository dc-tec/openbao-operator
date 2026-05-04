---
title: Publish a production service catalog
description: Add hardened implementation profiles, backup policy, bounded tenant parameters, and lifecycle defaults to a service-claim catalog.
slug: /service-claims/publish-production-catalog
hide_title: true
pageType: task
journey: get-started
---

<PageHeader
  title="Publish a production service catalog"
  lede="Production catalogs keep workload wiring in platform-owned profiles. Tenant users still select a stable offering and provide only bounded parameters that the selected catalog objects allow."
/>

<Checklist
  title="Prerequisites"
  items={[
    'publish or review the minimum catalog path first',
    'confirm the target service shape is supported by the catalog matrix',
    'choose the storage classes, unseal provider, backup target, edge posture, and upgrade defaults the platform will operate',
    'create any referenced provider credentials or workload-identity bindings outside the catalog objects',
  ]}
/>

<Callout type="note" title="Production catalog objects are policy">

Profiles in this page are cluster-scoped platform objects. Publish new
revisions when the contract changes instead of mutating existing production
profiles in place.

</Callout>

## Choose production policy homes

<DecisionTable
  title="Where production decisions belong"
  columns={['Decision', 'Catalog object', 'Tenant-facing surface']}
  rows={[
    {
      cells: [
        'OpenBao version, voter count, read replicas, security profile, and capacity',
        '`OpenBaoServiceProfile`',
        'Offering name only.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Storage classes and ACME shared cache storage',
        '`OpenBaoStorageProfile`',
        'No raw storage parameters.',
      ],
    },
    {
      cells: [
        'Unseal provider and credential posture',
        '`OpenBaoUnsealProfile`',
        'No tenant-selected provider credentials.',
      ],
    },
    {
      cells: [
        'ServiceAccount, helper images, hardening, and image policy',
        '`OpenBaoRuntimeProfile`',
        'No tenant-owned pod template.',
      ],
    },
    {
      cells: [
        'Metrics, ServiceMonitor, telemetry, DNS, API server, and egress dependencies',
        '`OpenBaoObservabilityProfile` and `OpenBaoNetworkProfile`',
        'No raw telemetry or NetworkPolicy snippets.',
      ],
    },
    {
      cells: [
        'Backup backend, credentials, key layout, retention, and restore source boundary',
        '`OpenBaoBackupProfile`, `OpenBaoBackupTarget`, `OpenBaoBackupBackend`, `OpenBaoBackupAuthProfile`, and `OpenBaoTransferProfile`',
        'Optional bounded backup partition when the target allows it.',
      ],
    },
    {
      cells: [
        'External endpoint, TLS, ACME, and tenant hostname bounds',
        '`OpenBaoExposureClass` and lower edge policy objects',
        'Optional hostname when the exposure class allows it.',
      ],
    },
  ]}
/>

## Add implementation profiles

Implementation profiles carry platform integration details that should not
appear on the claim.

<CommandBlock
  language="yaml"
  label="configure"
  title="Add reusable production profiles"
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
    init: registry.example.com/openbao-init:2.5.0
    backup: registry.example.com/openbao-backup:2.5.0
    restore: registry.example.com/openbao-backup:2.5.0
    upgrade: registry.example.com/openbao-upgrade:2.5.0
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoNetworkProfile
metadata:
  name: production-network-v1
spec:
  dnsNamespace: kube-system
  egressRules:
    - to:
        - ipBlock:
            cidr: 10.0.0.0/8
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

## Add backup policy

Backups use a layered catalog model so the service profile can reference a
stable backup profile while backend connectivity, credentials, transfer tuning,
and key layout remain separate platform-owned objects.

<CommandBlock
  language="yaml"
  label="configure"
  title="Publish object-storage backup policy"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoBackupBackend
metadata:
  name: object-storage-s3-v1
spec:
  driver: ObjectStorage
  objectStorage:
    provider: s3
    endpoint: https://s3.example.com
    region: eu-west-1
    usePathStyle: true
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoBackupAuthProfile
metadata:
  name: object-storage-static-v1
spec:
  mode: StaticCredentials
  staticCredentials:
    secretName: openbao-backup-credentials
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoTransferProfile
metadata:
  name: object-storage-transfer-v1
spec:
  partSize: 10485760
  concurrency: 3
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoBackupTarget
metadata:
  name: object-storage-target-v1
spec:
  backendRef:
    name: object-storage-s3-v1
  authProfileRef:
    name: object-storage-static-v1
  transportProfileRef:
    name: object-storage-transfer-v1
  locationPolicy:
    location:
      mode: Fixed
      value: openbao-backups
    keyPrefix:
      template: claims/{{ claim.namespace }}/{{ claim.name }}
      allowClaimPartition: true
  policy:
    deletionBehavior: Retain
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoBackupProfile
metadata:
  name: object-storage-scheduled-v1
spec:
  schedule: "0 */6 * * *"
  retention:
    maxCount: 28
    maxAge: 168h
  targetRef:
    name: object-storage-target-v1`}
>
  Set `allowClaimPartition: true` only when tenants may provide `spec.serviceParameters.backup.partition`. The location and key template still stay platform-owned.
</CommandBlock>

## Add bounded exposure

Use the exposure class to control publication mode, TLS, ACME, and any
tenant-provided hostname bounds.

<CommandBlock
  language="yaml"
  label="configure"
  title="Allow bounded tenant hostnames"
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
  Native OpenBao ACME uses the RWX cache storage selected by `OpenBaoStorageProfile.spec.acmeCache`.
</CommandBlock>

## Publish the production offering

Reference the production implementation profiles from the immutable service
profile. Tenants still select only the stable offering name.

<CommandBlock
  language="yaml"
  label="configure"
  title="Publish the hardened service profile and offering"
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

<CommandBlock
  language="bash"
  label="apply"
  title="Apply the checked-in production catalog sample"
  code={`kubectl apply -f config/samples/claims/dev-internal-catalog.yaml
kubectl apply -f config/samples/claims/hardened-edge-catalog.yaml`}
>
  The production sample reuses the `bootstrap-basic-v1` profile from the minimum catalog sample. Replace storage classes, provider endpoints, credentials, and image names before using it outside a test environment.
</CommandBlock>

## Hand tenants the bounded request surface

Tenant-provided values stay inside `spec.serviceParameters` and only work when
the selected catalog object permits them.

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
>
  Do not add raw storage, runtime, unseal, or network fields to the claim. Use profiles when the platform needs another supported production variant.
</CommandBlock>

<NextActions
  title="Continue production catalog work"
  items={[
    {
      label: 'Request a service',
      description: 'Create the tenant-facing claim once the production offering is published.',
      docId: 'user-guide/service-claims/getting-started',
    },
    {
      label: 'Plan exposure',
      description: 'Review ingress, gateway, TLS, ACME, and bounded hostname behavior.',
      docId: 'user-guide/service-claims/exposure',
    },
    {
      label: 'Operate claim services',
      description: 'Use immutable request objects for upgrades, manual backups, and restore.',
      docId: 'user-guide/service-claims/day-2-workflows',
    },
  ]}
/>
