---
description: Step-by-step recipe for a Hardened OpenBao cluster on Amazon EKS with AWS KMS auto-unseal, Gateway API passthrough, OpenBao-managed ACME, and S3 backups.
---

# Amazon EKS Hardened with AWS KMS, Gateway API Passthrough, and ACME

This recipe deploys a production-style `OpenBaoCluster` on Amazon EKS with:

- `spec.profile: Hardened`
- AWS KMS auto-unseal
- `spec.tls.mode: ACME`
- Gateway API TLS passthrough on a dedicated public edge
- JWT bootstrap for the Operator and a human admin `ServiceAccount`
- S3 backups with a separate backup identity

!!! success "Validated in the Manual EKS Environment"
    This recipe matches the hardened Amazon EKS lane validated in the project test environment. That path validated KMS unseal, OpenBao-managed ACME certificate issuance from Let's Encrypt, Gateway API passthrough, JWT bootstrap, and successful S3 backups.

!!! warning "Public reachability is required for public ACME"
    A public ACME CA such as Let's Encrypt must reach the hardened hostname on port `443`. Do not source-restrict the hardened passthrough endpoint to a single client IP. If you want ArgoCD or monitoring endpoints to stay restricted, keep them on a separate terminating edge.

!!! note "Reference architecture"
    For the tested topology, invariants, and validation scope behind this deployment flow, see [Amazon EKS Hardened with AWS KMS, Gateway API Passthrough, and ACME](../../architectures/cloud/amazon-eks-hardened-awskms-acme.md).

## Prerequisites

- Run Amazon EKS with an IAM OIDC provider enabled for IRSA or an equivalent workload identity setup.
- Install OpenBao Operator in multi-tenant mode with admission policies enabled.
- Create an AWS KMS key for auto-unseal and grant the main OpenBao workload access to it.
- Create an S3 bucket for Raft snapshots and a separate AWS identity for backup Jobs.
- Provide a `StorageClass` for the data PVCs and an RWX-capable `StorageClass` for the ACME shared cache, for example EFS CSI.
- Expose a dedicated public Gateway API passthrough edge for the hardened hostname.
- Ensure the selected Gateway controller supports `TLSRoute`. For Traefik, enable the Gateway API experimental channel.
- Publish public DNS for the hardened hostname.

## Inputs

Replace these values before applying the manifests:

| Placeholder | Example | Purpose |
| :--- | :--- | :--- |
| `<namespace>` | `openbaocluster-hardened` | Tenant namespace for the cluster |
| `<cluster-name>` | `openbaocluster-hardened` | `OpenBaoCluster` name |
| `<openbao-version>` | `2.5.1` | OpenBao version |
| `<aws-region>` | `eu-central-1` | AWS region for KMS and S3 |
| `<kms-key-arn>` | `arn:aws:kms:eu-central-1:123456789012:key/abcd...` | KMS key used for auto-unseal |
| `<main-role-arn>` | `arn:aws:iam::123456789012:role/openbao-unseal` | IRSA role for the main OpenBao Pods |
| `<backup-role-arn>` | `arn:aws:iam::123456789012:role/openbao-backup` | IRSA role for backup Jobs |
| `<backup-bucket>` | `openbao-backups` | S3 bucket for snapshots |
| `<external-host>` | `bao.example.com` | Public hostname for the hardened cluster |
| `<gateway-name>` | `openbao-hardened-gateway` | Dedicated passthrough Gateway |
| `<gateway-namespace>` | `default` | Namespace of the Gateway |
| `<gateway-class-name>` | `traefik-passthrough` | GatewayClass used by the dedicated passthrough edge |
| `<acme-cache-storage-class>` | `efs-acme` | RWX StorageClass for the shared ACME cache |
| `<operator-namespace>` | `openbao-operator-system` | Namespace hosting `OpenBaoTenant` |

## Step 1: Create the dedicated passthrough Gateway

Expose the hardened hostname through a dedicated public passthrough Gateway:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: <gateway-name>
  namespace: <gateway-namespace>
spec:
  gatewayClassName: <gateway-class-name>
  listeners:
    - name: websecure-passthrough
      hostname: <external-host>
      port: 443
      protocol: TLS
      tls:
        mode: Passthrough
      allowedRoutes:
        namespaces:
          from: All
```

!!! note "Validated edge shape"
    The validated EKS design used a dedicated Traefik release for the hardened hostname with:

    - a public `LoadBalancer`
    - only port `443` exposed
    - `externalTrafficPolicy: Local`
    - `TLSRoute` support enabled

    Keep this edge separate from any shared terminating Gateway used for ArgoCD, Grafana, or other admin UIs.

## Step 2: Create the tenant namespace

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

## Step 3: Apply the OpenBaoCluster

Apply the cluster manifest:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: <cluster-name>
  namespace: <namespace>
spec:
  profile: Hardened
  replicas: 3
  version: "<openbao-version>"

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

  imageVerification:
    enabled: true
    failurePolicy: Block
  operatorImageVerification:
    enabled: true
    failurePolicy: Block

  tls:
    enabled: true
    mode: ACME
    acme:
      directoryURL: "https://acme-v02.api.letsencrypt.org/directory"
      domains:
        - "<external-host>"
      email: "platform@example.com"
      sharedCache:
        mode: ManagedPVC
        size: "1Gi"
        storageClassName: <acme-cache-storage-class>

  storage:
    size: "10Gi"
    storageClassName: gp3
  deletionPolicy: DeleteAll

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
    listenerName: websecure-passthrough
    gatewayRef:
      name: <gateway-name>
      namespace: <gateway-namespace>
    hostname: "<external-host>"
    tlsPassthrough: true

  backup:
    schedule: "0 */6 * * *"
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

  network:
    egressRules:
      - to:
          - ipBlock:
              cidr: 0.0.0.0/0
        ports:
          - protocol: TCP
            port: 443
```

!!! tip "Helper image defaults"
    For released operator builds, prefer the default operator-managed helper images or explicitly pin official signed helper images that match your operator release.

## Operations

### Verify the cluster is ready

Check the status conditions:

```bash
kubectl -n <namespace> get openbaocluster <cluster-name> \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

The steady-state expectation is:

- `Available=True`
- `ACMEIntegrationReady=True`
- `ACMECacheReady=True`
- `CloudUnsealIdentityReady=True`
- `BackupConfigurationReady=True`
- `ProductionReady=True`
- `OpenBaoInitialized=True`
- `OpenBaoSealed=False`

### Verify Gateway programming

Check the Gateway directly:

```bash
kubectl -n <gateway-namespace> get gateway <gateway-name> -o yaml
```

Expect:

- `status.conditions` includes `Accepted=True`
- `status.conditions` includes `Programmed=True`

!!! note "GatewayIntegrationReady can remain Unknown"
    Some controllers do not publish enough `GatewayClass` status for the operator to conclude `GatewayIntegrationReady=True`. If the Gateway itself is accepted and programmed, and ACME succeeds, treat the route as operational.

### Verify the public certificate

Check the hardened endpoint:

```bash
curl -I https://<external-host>
```

Expect a valid public certificate and an OpenBao response such as `307`, `429`, or another application-level response.

### Verify JWT admin login

Create a Kubernetes token for the admin `ServiceAccount` and exchange it for an OpenBao token:

```bash
JWT="$(kubectl -n <namespace> create token openbao-admin --audience openbao-internal --duration=1h)"

curl -sS \
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

- ACME times out with `connection` or `secondary validation` errors: verify that the hardened hostname is publicly reachable on `443` and not IP-restricted.
- `ACMECacheReady=False`: verify that the shared cache PVC is RWX-capable and uses the intended StorageClass.
- `GatewayIntegrationReady=False`: verify the passthrough listener and `TLSRoute` support on the selected controller.
- `CloudUnsealIdentityReady=False`: verify the KMS key permissions and the IRSA annotation on the main workload.
- `BackupConfigurationReady=False`: verify the backup role ARN, S3 bucket access, and egress to the S3 endpoint.

## See Also

- [Gateway API Support](../../../openbaocluster/configuration/gateway-api.md)
- [Backups](../../../openbaocluster/operations/backups.md)
- [Troubleshooting](../../../openbaocluster/operations/troubleshooting.md)
