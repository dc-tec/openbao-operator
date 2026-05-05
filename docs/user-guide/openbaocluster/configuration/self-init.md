---
title: Self-Initialization
hide_title: true
pageType: task
journey: configure
description: Configure bootstrap requests, operator OIDC setup, and verification for self-initializing OpenBao clusters with an auto-revoked bootstrap credential.
---

<PageHeader
  title="Declarative cluster bootstrap"
  lede="Self-initialization applies auth methods, policies, secret engines, and other API bootstrap state from the `OpenBaoCluster` manifest and then revokes the bootstrap root token. Configure audit devices through `spec.audit` so they are rendered as OpenBao server configuration instead of API-created audit devices."
/>



<DecisionTable
  title="Choose the bootstrap path deliberately"
  columns={["Path", "Use it when", "What happens to the root token", "Watch for"]}
  rows={[
    {
      cells: [
        "Self-initialization",
        "You want declarative bootstrap and a production-ready baseline.",
        "The root token is auto-revoked after the requests complete successfully.",
        "You must define at least one usable auth path for humans or automation before the cluster comes up.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Standard init",
        "You need a temporary compatibility path for development or controlled manual setup.",
        "A root token can be created and stored in a Secret.",
        "This is easier to start with, but it leaves you with a stronger credential-management burden afterward.",
      ],
      emphasis: "caution",
    },
  ]}
/>

<DiagramFrame
  title="Self-init bootstrap flow"
  caption="The cluster initializes, applies the declared requests, and then revokes the bootstrap credential instead of treating it as a permanent operating dependency."
  code={`flowchart LR
    Cluster["OpenBaoCluster"] --> ServerConfig["Render server config"]
    ServerConfig --> Audit["spec.audit devices"]
    ServerConfig --> Init["Cluster initializes"]
    Init --> Requests["Apply selfInit requests"]
    Requests --> Auth["Auth methods, policies, engines"]
    Requests --> Revoke["Revoke root token"]
    Revoke --> Ready["Cluster ready for day 2"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Cluster,ServerConfig,Init read;
    class Requests,Revoke process;
    class Auth,Audit,Ready write;`}
/>

<Callout type="warning" title="Plan the access path before enabling self-init">

If self-init is enabled and no usable auth method exists for operators or humans, the root token is revoked and the cluster can become effectively unreachable unless it is recreated.

</Callout>

<DecisionTable
  title="Bootstrap both access surfaces together"
  columns={["Access surface", "Where it lives", "What must be true before first reconcile"]}
  rows={[
    {
      cells: [
        "Operator lifecycle auth",
        "`spec.selfInit.oidc.enabled`",
        "Enable it when you want the operator to bootstrap JWT auth for backup, restore, and upgrade work.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Human login path",
        "`spec.selfInit.requests`",
        "Create at least one usable auth method and policy path for people before the root token is revoked.",
      ],
    },
  ]}
/>

<Callout type="tip" title="Define operator and human access in the same bootstrap contract">

If the cluster will self-initialize, define the human login path in `selfInit.requests` as part of the same manifest that enables operator auth.

</Callout>

## Enable self-init

<CommandBlock
  language="yaml"
  label="configure"
  title="Pair self-init with declarative audit"
  code={`spec:
  audit:
    - type: file
      path: stdout
      fileOptions:
        filePath: stdout
  selfInit:
    enabled: true
    requests:
      - name: enable-userpass-auth
        operation: update
        path: sys/auth/userpass
        authMethod:
          type: userpass`}
>
  `spec.audit` defines audit devices in server configuration. `selfInit.requests` defines bootstrap API calls such as auth methods, policies, and secret engines.
</CommandBlock>

<CommandBlock
  language="yaml"
  label="configure"
  title="Pair operator OIDC bootstrap with a human auth path"
  code={`spec:
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
      - name: create-admin-policy
        operation: update
        path: sys/policies/acl/admin
        policy:
          policy: |
            path "*" {
              capabilities = ["create", "read", "update", "delete", "list", "sudo"]
            }
      # Add your user, JWT role, or Kubernetes auth role here so a real human
      # login path exists before the root token is revoked.`}
>
  The exact human auth method is your choice, but it belongs inside the same `selfInit` contract. For a complete worked example, see the [development self-init userpass recipe](../../validated-deployments/recipes/local/development-self-init-userpass.md).
</CommandBlock>

## What belongs in `requests`

<DecisionTable
  kind="reference"
  title="Structured request surfaces"
  columns={["Surface", "Use it for", "Typical example"]}
  rows={[
    {
      cells: [
        "`authMethod`",
        "Enable and configure auth backends.",
        "JWT or Kubernetes auth for operators and clients.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "`policy`",
        "Create ACL policies that your auth methods will bind to.",
        "Policies for apps, operators, or bootstrap-only roles.",
      ],
    },
    {
      cells: [
        "`secretEngine`",
        "Enable mounts such as KV or transit.",
        "Initial application secret storage or cryptography services.",
      ],
    },
    {
      cells: [
        "`data`",
        "Fallback for raw API payloads when no structured field exists.",
        "Specialized configuration that is not covered by the higher-level request fields yet.",
      ],
      emphasis: "caution",
    },
  ]}
/>

<Callout type="danger" title="Do not embed raw secrets in requests">

Avoid placing passwords, tokens, or key material directly in the manifest. Use Kubernetes Secrets where supported and treat bootstrap content like the rest of your GitOps security surface.

</Callout>

<Callout type="warning" title="Configure audit devices with spec.audit">

OpenBao rejects API-created audit devices unless `unsafeAllowAPIAuditCreation` is enabled. Keep audit devices in `spec.audit`; reserve `selfInit.requests[].auditDevice` for exceptional compatibility cases where you have deliberately enabled that unsafe OpenBao option.

</Callout>

## Bootstrap operator OIDC roles

<CommandBlock
  language="yaml"
  label="configure"
  title="Enable operator OIDC bootstrap"
  code={`spec:
  selfInit:
    enabled: true
    oidc:
      enabled: true
      # Optional:
      # issuer: "https://..."
      # audience: "openbao-internal"`}
>
  This bootstraps operator-only JWT auth roles for lifecycle work such as backup, upgrade, and restore. It does not create human login paths by itself.
</CommandBlock>

<DecisionTable
  kind="reference"
  title="What must stay aligned"
  columns={["Surface", "Why it matters", "What to align"]}
  rows={[
    {
      cells: [
        "OIDC issuer and JWKS discovery",
        "The operator must discover the Kubernetes issuer and keys to bootstrap JWT auth cleanly.",
        "Ensure the operator ServiceAccount can GET the OIDC discovery and JWKS non-resource URLs.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "JWT audience",
        "The role binding inside OpenBao and the projected token audience must match.",
        "Keep `spec.selfInit.oidc.audience` aligned with the installation-scoped `OPENBAO_JWT_AUDIENCE` value.",
      ],
    },
    {
      cells: [
        "Rendered controller identity",
        "Custom namespace or name-prefix changes affect the ServiceAccount subject the JWT role expects.",
        "If you manage roles manually, bind them to the rendered controller ServiceAccount identity rather than to a guessed default.",
      ],
    },
  ]}
/>

## Common bootstrap and baseline patterns

<Tabs groupId="self-init-patterns">
  <TabItem value="auth-method" label="Auth method">

<CommandBlock
  language="yaml"
  label="configure"
  title="Enable a JWT auth method"
  code={`- name: enable-jwt
  operation: update
  path: sys/auth/jwt-operator
  authMethod:
    type: jwt
    description: "Kubernetes JWT auth"
    config:
      default_lease_ttl: "1h"
      max_lease_ttl: "24h"`}
/>

  </TabItem>
  <TabItem value="policy" label="Policy">

<CommandBlock
  language="yaml"
  label="configure"
  title="Create an ACL policy at bootstrap"
  code={`- name: app-policy
  operation: update
  path: sys/policies/acl/app-policy
  policy:
    policy: |
      path "secret/data/app/*" {
        capabilities = ["read", "list"]
      }`}
/>

  </TabItem>
  <TabItem value="secret-engine" label="Secret engine">

<CommandBlock
  language="yaml"
  label="configure"
  title="Enable a KV v2 mount"
  code={`- name: enable-kv-v2
  operation: update
  path: sys/mounts/secret
  secretEngine:
    type: kv
    description: "General purpose KV store"
    options:
      version: "2"`}
/>

  </TabItem>
  <TabItem value="audit-device" label="Audit baseline">

<CommandBlock
  language="yaml"
  label="configure"
  title="Enable audit logging declaratively"
  code={`spec:
  audit:
    - type: file
      path: file
      fileOptions:
        filePath: /var/log/openbao/audit.log`}
/>

  </TabItem>
</Tabs>

## Verify the cluster finished bootstrap

<CommandBlock
  language="bash"
  label="verify"
  title="Check the self-init status bit"
  code={`kubectl get openbaocluster <name> -o jsonpath='{.status.selfInitialized}'`}
>
  A healthy bootstrap should report `true`. If it does not, inspect the cluster status conditions and controller logs before retrying with additional requests.
</CommandBlock>

<NextActions
  title="Continue cluster baseline"
  items={[
    {
      label: "Server configuration",
      description: "Move from bootstrap into the steady-state server settings and autopilot defaults you want to keep.",
      docId: "user-guide/openbaocluster/configuration/server",
    },
    {
      label: "Operator authentication",
      description: "Review the operator-side auth contract if you are relying on OIDC bootstrap.",
      docId: "user-guide/operator/authn",
    },
    {
      label: "Backup operations",
      description: "See how the operator OIDC role is used by later lifecycle workflows.",
      docId: "user-guide/openbaocluster/operations/backups",
    },
  ]}
/>
