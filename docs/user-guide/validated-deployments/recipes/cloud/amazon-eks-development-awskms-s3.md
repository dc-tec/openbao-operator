---
description: Step-by-step recipe for a Development-profile OpenBao cluster on Amazon EKS with AWS KMS auto-unseal, JWT self-init, Gateway API exposure, and S3 backups.
---

# Amazon EKS Development with AWS KMS and S3 Backups

This recipe deploys a `Development`-profile `OpenBaoCluster` on Amazon EKS with:

- AWS KMS auto-unseal on the main OpenBao Pods
- JWT bootstrap for the Operator and a human admin `ServiceAccount`
- Gateway API exposure through a shared terminating edge
- scheduled backups to S3 using a separate backup identity

!!! success "Validated in the Manual EKS Environment"
    This recipe is based on the Amazon EKS lane used for manual validation in the project test environment. That path validated KMS unseal, JWT bootstrap, Gateway API exposure, and successful S3 backups.

!!! warning "Development only"
    Use this recipe for validation, demos, and operational bring-up. `spec.profile: Development` is not a production posture.

!!! note "Reference architecture"
    For the tested topology, assumptions, and validation scope behind this deployment flow, see [Amazon EKS Development with AWS KMS and S3 Backups](../../architectures/cloud/amazon-eks-development-awskms-s3.md).

## Prerequisites

- Run Amazon EKS with an IAM OIDC provider enabled for IRSA or an equivalent workload identity setup.
- Install OpenBao Operator in multi-tenant mode with admission policies enabled.
- Create an AWS KMS key for auto-unseal and grant the main OpenBao workload access to it.
- Create an S3 bucket for Raft snapshots.
- Create separate AWS identities for:
  - the main OpenBao Pods (KMS unseal)
  - backup Jobs (S3 write access)
- Expose a Gateway API listener that terminates HTTPS for the external hostname and can re-encrypt to the OpenBao backend.
- Provision a certificate for the shared edge hostname outside the `OpenBaoCluster`, for example with cert-manager and Route53 DNS01.
- Ensure a `StorageClass` exists for the data PVCs.

## Inputs

Replace these values before applying the manifests:

| Placeholder | Example | Purpose |
| :--- | :--- | :--- |
| `<namespace>` | `openbaocluster-dev` | Tenant namespace for the cluster |
| `<cluster-name>` | `openbaocluster-dev` | `OpenBaoCluster` name |
| `<openbao-version>` | `2.5.1` | OpenBao version |
| `<aws-region>` | `eu-central-1` | AWS region for KMS and S3 |
| `<kms-key-arn>` | `arn:aws:kms:eu-central-1:123456789012:key/abcd...` | KMS key used for auto-unseal |
| `<main-role-arn>` | `arn:aws:iam::123456789012:role/openbao-unseal` | IRSA role for the main OpenBao Pods |
| `<backup-role-arn>` | `arn:aws:iam::123456789012:role/openbao-backup` | IRSA role for backup Jobs |
| `<backup-bucket>` | `openbao-backups` | S3 bucket for snapshots |
| `<gateway-name>` | `shared-gateway` | Existing terminating Gateway |
| `<gateway-namespace>` | `default` | Namespace of the Gateway |
| `<external-host>` | `bao-dev.example.com` | External hostname for the development cluster |
| `<operator-namespace>` | `openbao-operator-system` | Namespace hosting `OpenBaoTenant` |

## Step 1: Create the tenant namespace

Apply the tenant namespace, onboarding request, and admin `ServiceAccount`:

```yaml
apiVersion: v1
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
  namespace: <namespace>
```

Verify that the tenant is provisioned:

```bash
kubectl -n <operator-namespace> describe openbaotenant <cluster-name>-tenant
```

## Step 2: Apply the OpenBaoCluster

Apply the cluster manifest:

```yaml
apiVersion: openbao.org/v1alpha1
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
    strategy: RollingUpdate
```

!!! note "AppArmor on EKS"
    The validated EKS lane set `spec.workloadHardening.appArmorEnabled: false`. If your node OS supports AppArmor cleanly, remove that override.

!!! note "Separate identity surfaces"
    `spec.serviceAccount.annotations` configures the main OpenBao Pods. The backup path uses its own generated ServiceAccount and the S3 identity configured under `spec.backup.target`.

## Operations

### Verify the cluster is ready

Check the status conditions:

```bash
kubectl -n <namespace> get openbaocluster <cluster-name> \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

The steady-state expectation is:

- `Available=True`
- `CloudUnsealIdentityReady=True`
- `BackupConfigurationReady=True`
- `OpenBaoInitialized=True`
- `OpenBaoSealed=False`

`ProductionReady` is not the goal for this `Development` recipe.

If you enabled Gateway exposure, also expect `GatewayIntegrationReady=True` or `Unknown` while the controller reports status.

### Verify the external endpoint

Check the external health path:

```bash
curl -kI https://<external-host>/v1/sys/health
```

Expect an OpenBao response such as `200`, `429`, or `472` depending on your health-query parameters.

### Verify JWT admin login

Create a Kubernetes token for the admin `ServiceAccount` and exchange it for an OpenBao token:

```bash
JWT="$(kubectl -n <namespace> create token openbao-admin --audience openbao-internal --duration=1h)"

curl -sS -k \
  -H 'Content-Type: application/json' \
  -d "{\"role\":\"admin\",\"jwt\":\"${JWT}\"}" \
  "https://<external-host>/v1/auth/jwt/login"
```

### Trigger a manual backup

Patch the cluster with the supported manual-backup annotation:

```bash
kubectl -n <namespace> annotate openbaocluster <cluster-name> \
  openbao.org/trigger-backup="$(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite
```

Then inspect backup status:

```bash
kubectl -n <namespace> get openbaocluster <cluster-name> \
  -o jsonpath='{.status.backup.lastBackupName}{"\n"}{.status.backup.lastBackupTime}{"\n"}{.status.backup.lastFailureReason}{"\n"}'
```

## Common Failures

- `CloudUnsealIdentityReady=False`: verify the KMS key permissions and the IRSA annotation on the main workload.
- `BackupConfigurationReady=False`: verify the backup role ARN, S3 bucket access, and network egress to the S3 endpoint.
- `OpenBaoInitialized=False`: check `spec.selfInit.requests` for syntax or path errors.
- The Gateway returns edge `404`: verify that the referenced Gateway listener exists and your controller accepts the generated `HTTPRoute`.

## See Also

- [Gateway API Support](../../../openbaocluster/configuration/gateway-api.md)
- [Backups](../../../openbaocluster/operations/backups.md)
- [Self-Initialization](../../../openbaocluster/configuration/self-init.md)
