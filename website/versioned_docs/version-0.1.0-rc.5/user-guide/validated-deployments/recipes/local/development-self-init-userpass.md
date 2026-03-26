---
title: k3d Development / Shared Edge Recipe
hide_title: true
pageType: task
journey: validated-deployments
description: Reproduce the validated local development lane with self-init, operator-managed TLS, a shared terminating edge, demo login, JWT admin access, and RustFS backups.
---

<PageHeader
  title="Reproduce the validated local development lane without turning a quick-start cluster into a pile of one-off overrides."
  lede="This recipe stands up the local development baseline with tenant onboarding, operator-managed TLS, a shared terminating edge, JWT bootstrap, an optional demo login, and an S3-compatible backup path backed by RustFS."
/>

<Callout type="success" title="Validated lane">

This recipe follows the development lifecycle, backup, and blue/green patterns exercised in the local validation environment and the in-repo E2E suites. The optional `userpass` login remains local-only convenience layered on top of the validated cluster path.

</Callout>

<Callout type="note" title="Use the main docs for the product-wide source of truth">

Use <SiteLink docId="user-guide/openbaotenant/onboarding">tenant onboarding</SiteLink>, <SiteLink docId="user-guide/openbaocluster/configuration/external-access">external access</SiteLink>, and <SiteLink docId="user-guide/openbaocluster/operations/backups">backup operations</SiteLink> when you need the generic operator behavior. This recipe only captures the exact validated local lane.

</Callout>

<DecisionTable
  title="What this lane assumes"
  columns={["Assumption", "Why it exists", "What breaks if it is wrong"]}
  rows={[
    {
      cells: [
        "Multi-tenant operator install with admission enabled",
        "The validated path starts from the default tenant-onboarding model.",
        "Namespace provisioning and generated RBAC will drift from the lane you are trying to reproduce.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "A shared terminating Gateway already exists",
        "The lane intentionally reuses one local edge for OpenBao and the rest of the toolchain.",
        "You will not validate the same routing contract if you fall back to port-forwarding or user-managed passthrough.",
      ],
    },
    {
      cells: [
        "RustFS is reachable as an S3-compatible endpoint",
        "Backups are part of the lane, not an optional afterthought.",
        "The cluster may look healthy while the part of the lane that matters for restore rehearsal never actually works.",
      ],
    },
    {
      cells: [
        "Demo login stays local-only",
        "The `userpass` bootstrap is included only to make UI validation and demos faster.",
        "Reusing it outside a disposable environment turns a convenience into a security mistake.",
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
    {cells: ["`<namespace>`", "`openbaocluster-demo`", "Tenant namespace for the cluster."]},
    {cells: ["`<cluster-name>`", "`openbaocluster-demo`", "`OpenBaoCluster` name."]},
    {cells: ["`<openbao-version>`", "`2.5.1`", "OpenBao version."]},
    {cells: ["`<gateway-name>`", "`shared-gateway`", "Existing terminating Gateway used by the local toolchain."]},
    {cells: ["`<gateway-namespace>`", "`default`", "Namespace of the Gateway."]},
    {cells: ["`<external-host>`", "`bao-demo.example.com`", "External hostname for the shared-edge route."]},
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
  The steady-state expectation is `Provisioned=True`. Do not move to the cluster manifest until the namespace is actually prepared for the operator.
</CommandBlock>

## Step 2: Create the backup credentials Secret

<CommandBlock
  language="bash"
  label="apply"
  title="Create the RustFS credentials Secret"
  code={`kubectl -n <namespace> create secret generic rustfs-secret \\
  --from-literal=accessKeyId='rustfsadmin' \\
  --from-literal=secretAccessKey='rustfsadmin'`}
>
  If your local RustFS instance uses different credentials, replace both values here and in the object-storage service itself.
</CommandBlock>

## Step 3: Apply the validated development cluster manifest

<CommandBlock
  language="yaml"
  label="apply"
  title="Apply the Development-profile cluster"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: <cluster-name>
  namespace: <namespace>
spec:
  profile: Development
  replicas: 3
  version: "<openbao-version>"

  tls:
    enabled: true
    mode: OperatorManaged
    rotationPeriod: "720h"

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

  storage:
    size: "10Gi"
  deletionPolicy: DeleteAll

  selfInit:
    enabled: true
    oidc:
      enabled: true
    requests:
      - name: enable-userpass-auth
        operation: update
        path: sys/auth/userpass
        authMethod:
          type: userpass
      - name: enable-jwt-auth
        operation: update
        path: sys/auth/jwt
        authMethod:
          type: jwt
      - name: enable-demo-kv
        operation: update
        path: sys/mounts/secret
        secretEngine:
          type: kv
          description: "Demo KV v2 engine"
          options:
            version: "2"
      - name: create-admin-policy
        operation: update
        path: sys/policies/acl/admin
        policy:
          policy: |
            path "*" {
              capabilities = ["create", "read", "update", "delete", "list", "sudo"]
            }
      - name: create-demo-ui-user
        operation: update
        path: auth/userpass/users/demo-admin
        data:
          password: "demo-password"
          token_policies:
            - admin
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
      endpoint: "http://rustfs-svc.rustfs.svc.cluster.local:9000"
      bucket: "openbao-backups"
      pathPrefix: "clusters/<cluster-name>"
      usePathStyle: true
      credentialsSecretRef:
        name: rustfs-secret
    retention:
      maxCount: 7
      maxAge: "168h"

  upgrade:
    preUpgradeSnapshot: true
    strategy: BlueGreen`}
/>

<Callout type="warning" title="Demo-only credentials">

The `demo-admin` user exists only to make local validation easy. Keep it out of any shared or long-lived environment.

</Callout>

<Callout type="note" title="AppArmor on local clusters">

If kubelet rejects the Pods because AppArmor is unavailable, add:

```yaml
spec:
  workloadHardening:
    appArmorEnabled: false
```

</Callout>

## Verify the lane

<CommandBlock
  language="bash"
  label="verify"
  title="Check the cluster conditions"
  code={`kubectl -n <namespace> get openbaocluster <cluster-name> \\
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\\n"}{end}'`}
>
  The steady-state expectation is `Available=True`, `TLSReady=True`, `UserAccessBootstrap=True`, `BackupConfigurationReady=True`, `GatewayIntegrationReady=True`, `OpenBaoInitialized=True`, and `OpenBaoSealed=False`.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Confirm the cluster did not persist a root token Secret"
  code={`kubectl -n <namespace> get secret <cluster-name>-root-token`}
>
  This should return `NotFound`. A self-init lane should not leave the root token stored as a Kubernetes Secret.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Verify the demo UI login through the local service"
  code={`kubectl -n <namespace> port-forward svc/<cluster-name> 8200:8200
export VAULT_ADDR="https://127.0.0.1:8200"

curl -sS -k \\
  -H 'Content-Type: application/json' \\
  -d '{"password":"demo-password"}' \\
  \${VAULT_ADDR%/}/v1/auth/userpass/login/demo-admin`}
>
  Local browsers and CLIs may warn about the operator-managed CA. That is expected in this lane.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Verify JWT admin login"
  code={`JWT="$(kubectl -n <namespace> create token openbao-admin --audience openbao-internal --duration=1h)"

curl -sS -k \\
  -H 'Content-Type: application/json' \\
  -d "{\\"role\\":\\"admin\\",\\"jwt\\":\\"\${JWT}\\"}" \\
  \${VAULT_ADDR%/}/v1/auth/jwt/login`}
/>

<CommandBlock
  language="bash"
  label="verify"
  title="Trigger and inspect a manual backup"
  code={`kubectl -n <namespace> annotate openbaocluster <cluster-name> \\
  openbao.org/trigger-backup="$(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite

kubectl -n <namespace> get openbaocluster <cluster-name> \\
  -o jsonpath='{.status.backup.lastBackupName}{"\\n"}{.status.backup.lastBackupTime}{"\\n"}{.status.backup.lastFailureReason}{"\\n"}'`}
>
  A successful lane should produce a backup object key and no failure reason.
</CommandBlock>

<NextActions
  title="Keep moving"
  items={[
    {
      label: "Reference architecture",
      description: "Review the lane summary, topology, and invariants behind the recipe you just applied.",
      docId: "user-guide/validated-deployments/architectures/local/k3d-development-shared-edge-rustfs",
    },
    {
      label: "Backup operations",
      description: "Expand the RustFS-specific recipe choices into the generic backup model used by the operator.",
      docId: "user-guide/openbaocluster/operations/backups",
    },
    {
      label: "k3d Hardened / External TLS",
      description: "Move to the hardened local rehearsal lane when you need external certificates and an external unseal root.",
      docId: "user-guide/validated-deployments/architectures/local/k3d-hardened-transit-external-tls",
    },
  ]}
/>
