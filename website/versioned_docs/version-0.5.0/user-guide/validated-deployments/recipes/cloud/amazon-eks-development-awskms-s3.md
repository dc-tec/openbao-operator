---
title: EKS Development / Shared Edge Recipe
hide_title: true
pageType: task
journey: validated-deployments
description: Reproduce the validated development baseline on Amazon EKS with AWS KMS auto-unseal, a shared terminating Gateway, JWT bootstrap, and S3 backups.
---

<PageHeader
  title="Reproduce the validated EKS development lane"
  lede="This recipe applies the EKS development baseline with KMS auto-unseal, shared-edge exposure, JWT bootstrap, and S3 backups. Use it when you need the exact validated cloud bring-up path for this topology."
/>

<Checklist
    title="Recipe outcomes"
    items={[
      "an onboarded tenant namespace and admin ServiceAccount",
      "a Development-profile cluster that unseals with AWS KMS through workload identity",
      "JWT admin login and shared-edge access working on the public hostname you chose",
      "manual and scheduled S3 backups using a backup identity that stays separate from the main workload identity",
    ]}
  />


<Callout type="success" title="Validated coverage">

This recipe matches the EKS development lane validated in the project cloud environment. The tested path covered KMS unseal, JWT bootstrap, Gateway exposure, and successful S3 backups.

</Callout>

<Callout type="note" title="Use the main docs for generic operator behavior">

Use <SiteLink docId="user-guide/openbaotenant/onboarding">tenant onboarding</SiteLink>, <SiteLink docId="user-guide/openbaocluster/configuration/gateway-api">Gateway API support</SiteLink>, and <SiteLink docId="user-guide/openbaocluster/operations/backups">backup operations</SiteLink> for the product-wide guidance. This recipe documents the validated lane.

</Callout>

<DecisionTable
  title="Baseline assumptions"
  columns={["Assumption", "Why it exists", "What breaks if it is wrong"]}
  rows={[
    {
      cells: [
        "EKS has IRSA or an equivalent workload identity path enabled",
        "Both KMS unseal and S3 backup identities depend on cloud workload identity behavior.",
        "The main workload or backup jobs will fail authentication even if the manifests themselves look correct.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "The shared Gateway terminates HTTPS and can re-encrypt to OpenBao",
        "The lane uses a terminating edge instead of passthrough.",
        "If you switch to passthrough or plain HTTP forwarding, you are no longer reproducing the validated baseline.",
      ],
    },
    {
      cells: [
        "Separate AWS identities exist for unseal and backup",
        "The validated lane treats those permissions as distinct surfaces.",
        "You will miss the exact failure modes and permission boundaries the lane is supposed to prove.",
      ],
    },
    {
      cells: [
        "This remains a Development profile",
        "The lane is intentionally optimized for bring-up and validation speed.",
        "Treating it as a production profile will create the wrong expectations around `ProductionReady` and endpoint posture.",
      ],
      emphasis: "caution",
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Inputs to replace before apply"
  columns={["Placeholder", "Example", "Purpose"]}
  rows={[
    {cells: ["`<namespace>`", "`openbaocluster-dev`", "Tenant namespace for the cluster."]},
    {cells: ["`<cluster-name>`", "`openbaocluster-dev`", "`OpenBaoCluster` name."]},
    {cells: ["`<openbao-version>`", "`2.5.1`", "OpenBao version."]},
    {cells: ["`<aws-region>`", "`eu-central-1`", "AWS region for KMS and S3."]},
    {cells: ["`<kms-key-arn>`", "`arn:aws:kms:...`", "KMS key ARN for auto-unseal."]},
    {cells: ["`<main-role-arn>`", "`arn:aws:iam::...:role/openbao-unseal`", "IRSA role for the main OpenBao Pods."]},
    {cells: ["`<backup-role-arn>`", "`arn:aws:iam::...:role/openbao-backup`", "IRSA role for backup Jobs."]},
    {cells: ["`<backup-bucket>`", "`openbao-backups`", "S3 bucket for snapshots."]},
    {cells: ["`<gateway-name>`", "`shared-gateway`", "Existing terminating Gateway."]},
    {cells: ["`<gateway-namespace>`", "`default`", "Namespace of the Gateway."]},
    {cells: ["`<external-host>`", "`bao-dev.example.com`", "External hostname for the development cluster."]},
    {cells: ["`<operator-namespace>`", "`openbao-operator-system`", "Namespace that hosts the central `OpenBaoTenant` resource."]},
  ]}
/>

## Step 1: Onboard the tenant namespace

<CommandBlock
  language="yaml"
  label="apply"
  title="Create the namespace, onboarding request, and admin ServiceAccount"
  code={`apiVersion: v1
kind: Namespace
metadata:
  name: <namespace>
  labels:
    openbao.org/tenant: "true"
---
apiVersion: openbao.org/v1alpha1
kind: OpenBaoTenant
metadata:
  name: <cluster-name>-tenant
  namespace: <operator-namespace>
spec:
  targetNamespace: <namespace>
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: openbao-admin
  namespace: <namespace>`}
/>

<CommandBlock
  language="bash"
  label="verify"
  title="Wait for tenant provisioning"
  code={`kubectl -n <operator-namespace> describe openbaotenant <cluster-name>-tenant`}
>
  The steady-state expectation is `Provisioned=True`.
</CommandBlock>

## Step 2: Apply the validated EKS development cluster

<CommandBlock
  language="yaml"
  label="apply"
  title="Apply the Development-profile EKS manifest"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: <cluster-name>
  namespace: <namespace>
spec:
  profile: Development
  replicas: 3
  version: "<openbao-version>"

  workloadHardening:
    appArmorEnabled: false

  configuration:
    logLevel: "info"
    ui: true
    logging:
      format: "json"
    defaultLeaseTTL: "720h"
    maxLeaseTTL: "8760h"
    cacheSize: 134217728
    disableCache: false
    raft:
      performanceMultiplier: 2

  tls:
    enabled: true
    mode: External

  storage:
    size: "10Gi"
    storageClassName: gp3
  deletionPolicy: DeleteAll

  imageVerification:
    enabled: false
    failurePolicy: Block
  operatorImageVerification:
    enabled: false
    failurePolicy: Block

  serviceAccount:
    annotations:
      eks.amazonaws.com/role-arn: "<main-role-arn>"

  unseal:
    type: awskms
    awskms:
      region: "<aws-region>"
      kmsKeyID: "<kms-key-arn>"

  selfInit:
    enabled: true
    oidc:
      enabled: true
    requests:
      - name: enable-jwt-auth
        operation: update
        path: sys/auth/jwt
        authMethod:
          type: jwt
      - name: create-admin-policy
        operation: update
        path: sys/policies/acl/admin
        policy:
          policy: |
            path "*" {
              capabilities = ["create", "read", "update", "delete", "list", "sudo"]
            }
      - name: create-admin-jwt-role
        operation: update
        path: auth/jwt/role/admin
        data:
          role_type: jwt
          user_claim: sub
          bound_audiences:
            - openbao-internal
          bound_subject: system:serviceaccount:<namespace>:openbao-admin
          token_policies:
            - admin
          policies:
            - admin
          ttl: 1h

  gateway:
    enabled: true
    listenerName: websecure
    gatewayRef:
      name: <gateway-name>
      namespace: <gateway-namespace>
    hostname: "<external-host>"
    backendTLS:
      enabled: true
    tlsPassthrough: false
    path: /

  backup:
    schedule: "*/30 * * * *"
    target:
      provider: s3
      endpoint: "https://s3.<aws-region>.amazonaws.com"
      bucket: "<backup-bucket>"
      pathPrefix: "clusters/<cluster-name>"
      region: "<aws-region>"
      roleArn: "<backup-role-arn>"
      usePathStyle: false
    retention:
      maxCount: 7
      maxAge: "168h"

  upgrade:
    preUpgradeSnapshot: true
    strategy: RollingUpdate`}
/>

<Callout type="note" title="AppArmor on EKS">

The validated EKS lane set `spec.workloadHardening.appArmorEnabled: false`. Remove that override only if your node OS and runtime support AppArmor cleanly.

</Callout>

## Verify the lane

<CommandBlock
  language="bash"
  label="verify"
  title="Check the cluster conditions"
  code={`kubectl -n <namespace> get openbaocluster <cluster-name> \\
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\\n"}{end}'`}
>
  The steady-state expectation is `Available=True`, `CloudUnsealIdentityReady=True`, `BackupConfigurationReady=True`, `GatewayIntegrationReady=True` or `Unknown`, `OpenBaoInitialized=True`, and `OpenBaoSealed=False`.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Check the external endpoint and JWT admin login"
  code={`curl -kI https://<external-host>/v1/sys/health

JWT="$(kubectl -n <namespace> create token openbao-admin --audience openbao-internal --duration=1h)"

curl -sS -k \\
  -H 'Content-Type: application/json' \\
  -d "{\\"role\\":\\"admin\\",\\"jwt\\":\\"\${JWT}\\"}" \\
  "https://<external-host>/v1/auth/jwt/login"`}
/>

<CommandBlock
  language="bash"
  label="verify"
  title="Trigger and inspect a manual backup"
  code={`kubectl -n <namespace> annotate openbaocluster <cluster-name> \\
  openbao.org/trigger-backup="$(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite

kubectl -n <namespace> get openbaocluster <cluster-name> \\
  -o jsonpath='{.status.backup.lastBackupName}{"\\n"}{.status.backup.lastBackupTime}{"\\n"}{.status.backup.lastFailureReason}{"\\n"}{.status.backup.lastFailureMessage}{"\\n"}'`}
/>

<NextActions
  title="Keep moving"
  items={[
    {
      label: "Reference architecture",
      description: "Review the lane summary, topology, and invariants behind the EKS development path.",
      docId: "user-guide/validated-deployments/architectures/cloud/amazon-eks-development-awskms-s3",
    },
    {
      label: "Backup operations",
      description: "Use the operator-wide backup guide for retention, credentials, and restore planning beyond this validated lane.",
      docId: "user-guide/openbaocluster/operations/backups",
    },
    {
      label: "EKS Hardened",
      description: "Move to the hardened cloud baseline when you are ready for ACME, passthrough, and a production-style edge.",
      docId: "user-guide/validated-deployments/architectures/cloud/amazon-eks-hardened-awskms-acme",
    },
  ]}
/>
