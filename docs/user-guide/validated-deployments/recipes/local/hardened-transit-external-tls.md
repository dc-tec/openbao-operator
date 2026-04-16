---
title: k3d Hardened / External TLS Recipe
hide_title: true
pageType: task
journey: validated-deployments
description: Reproduce the validated local hardened lane with Transit auto-unseal, externally managed TLS Secrets, self-init, and user-managed passthrough access.
---

<PageHeader
  title="Reproduce the validated hardened local lane"
  lede="This recipe stands up the local hardened lane with tenant onboarding, external Transit unseal, externally managed TLS Secrets, and user-managed TCP passthrough. Use it when you want the exact validated path for this topology."
/>

<Checklist
    title="Recipe outcomes"
    items={[
      "an onboarded tenant namespace and admin ServiceAccount",
      "a hardened cluster that self-initializes and never persists a root token Secret",
      "Transit auto-unseal working against the external provider you chose for the lane",
      "externally managed TLS Secrets and end-to-end passthrough traffic working together",
    ]}
  />


<Callout type="success" title="Validated coverage">

This recipe follows the hardened external-TLS lifecycle covered by the in-repo E2E suite and the local validation environment. The tested path includes tenant onboarding, external TLS Secrets, Transit auto-unseal, self-init, and successful JWT admin login.

</Callout>

<Callout type="note" title="Canonical operator behavior still lives in the main docs">

Use the main guides for the product-wide source of truth on <SiteLink docId="user-guide/openbaocluster/configuration/security-profiles">security profiles</SiteLink>, <SiteLink docId="user-guide/openbaotenant/onboarding">tenant onboarding</SiteLink>, and <SiteLink docId="user-guide/openbaocluster/configuration/external-access">external access</SiteLink>. This recipe only captures the exact validated lane.

</Callout>

<DecisionTable
  title="What this lane assumes"
  columns={["Assumption", "Why it exists", "What breaks if it is wrong"]}
  rows={[
    {
      cells: [
        "Multi-tenant operator install with admission enabled",
        "The validated lane starts from the standard tenant-onboarding path.",
        "Namespace onboarding and guarded hardened behavior will not match the tested path.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "cert-manager is available",
        "The lane expects external TLS Secrets to be provisioned before the workload depends on them.",
        "The cluster will not reach `TLSReady=True` if the Secrets never appear.",
      ],
    },
    {
      cells: [
        "Transit is reachable from the tenant namespace",
        "The lane keeps the seal root external on purpose.",
        "The cluster may initialize but fail to unseal or rejoin correctly after restart.",
      ],
    },
    {
      cells: [
        "You know the ingress namespace",
        "The network policy must trust the namespace that forwards passthrough traffic.",
        "Traffic may never reach the public Service even though the cluster itself is healthy.",
      ],
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Inputs to replace before apply"
  columns={["Placeholder", "Example", "Purpose"]}
  rows={[
    {cells: ["`<namespace>`", "`openbaocluster-hardened`", "Tenant namespace for the cluster."]},
    {cells: ["`<cluster-name>`", "`openbaocluster-hardened`", "`OpenBaoCluster` name."]},
    {cells: ["`<openbao-version>`", "`2.5.0`", "OpenBao version."]},
    {cells: ["`<transit-address>`", "`https://transit-provider.openbao-infra.svc:8200`", "Transit provider URL."]},
    {cells: ["`<transit-key>`", "`openbao-unseal`", "Transit key name."]},
    {cells: ["`<external-host>`", "`bao-hardened.example.com`", "External DNS name for clients."]},
    {cells: ["`<ingress-namespace>`", "`default`", "Namespace of the ingress controller that forwards traffic to OpenBao."]},
    {cells: ["`<transit-namespace>`", "`openbao-infra`", "Namespace hosting the Transit provider."]},
    {cells: ["`<operator-namespace>`", "`openbao-operator-system`", "Namespace hosting the central `OpenBaoTenant` resource."]},
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
  title="Verify tenant provisioning"
  code={`kubectl -n <operator-namespace> describe openbaotenant <cluster-name>-tenant`}
>
  The steady-state expectation is `Provisioned=True`. Do not continue until the tenant onboarding path is actually complete.
</CommandBlock>

## Step 2: Create the Transit credential Secret

<CommandBlock
  language="bash"
  label="apply"
  title="Create the Secret used by transit auto-unseal"
  code={`kubectl -n <namespace> create secret generic transit-provider-token \\
  --from-literal=token='<transit-token>' \\
  --from-file=ca.crt=/path/to/transit-provider-ca.crt`}
>
  For the validated path, the Secret contains `token` for `VAULT_TOKEN` and `ca.crt` for `VAULT_CACERT`.
</CommandBlock>

## Step 3: Create the external TLS Secrets

<CommandBlock
  language="yaml"
  label="apply"
  title="Issue the TLS CA and server Secrets with cert-manager"
  code={`apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: <cluster-name>-selfsigned-issuer
  namespace: <namespace>
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: <cluster-name>-tls-ca
  namespace: <namespace>
spec:
  secretName: <cluster-name>-tls-ca
  commonName: <cluster-name>-ca
  isCA: true
  issuerRef:
    kind: Issuer
    name: <cluster-name>-selfsigned-issuer
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: <cluster-name>-ca-issuer
  namespace: <namespace>
spec:
  ca:
    secretName: <cluster-name>-tls-ca
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: <cluster-name>-tls-server
  namespace: <namespace>
spec:
  secretName: <cluster-name>-tls-server
  dnsNames:
    - <external-host>
    - openbao-cluster-<cluster-name>.local
    - <cluster-name>.<namespace>.svc
    - "*.<cluster-name>.<namespace>.svc"
    - <cluster-name>-public.<namespace>.svc
  issuerRef:
    kind: Issuer
    name: <cluster-name>-ca-issuer`}
/>

<CommandBlock
  language="bash"
  label="verify"
  title="Wait for the TLS Secrets to become ready"
  code={`kubectl -n <namespace> wait certificate/<cluster-name>-tls-ca --for=condition=Ready --timeout=5m
kubectl -n <namespace> wait certificate/<cluster-name>-tls-server --for=condition=Ready --timeout=5m`}
>
  If you already use a corporate issuer, replace the issuer objects but keep the Secret names `<cluster-name>-tls-ca` and `<cluster-name>-tls-server`.
</CommandBlock>

## Step 4: Apply the OpenBaoCluster

<CommandBlock
  language="yaml"
  label="apply"
  title="Apply the validated hardened cluster manifest"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: <cluster-name>
  namespace: <namespace>
spec:
  profile: Hardened
  replicas: 3
  version: "<openbao-version>"

  storage:
    size: "10Gi"
  deletionPolicy: Retain

  tls:
    enabled: true
    mode: External

  configuration:
    logLevel: "info"
    ui: true
    logging:
      format: "json"

  unseal:
    type: transit
    credentialsSecretRef:
      name: transit-provider-token
    transit:
      address: "<transit-address>"
      mountPath: "transit"
      keyName: "<transit-key>"
      tlsCACert: "/etc/bao/seal-creds/ca.crt"

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

  upgrade:
    strategy: RollingUpdate

  network:
    trustedIngressPeers:
      - namespaceSelector:
          matchLabels:
            kubernetes.io/metadata.name: <ingress-namespace>
    egressRules:
      - to:
          - namespaceSelector:
              matchLabels:
                kubernetes.io/metadata.name: <transit-namespace>
        ports:
          - protocol: TCP
            port: 8200`}
/>

<Callout type="warning" title="API server endpoint IPs">

If your CNI enforces egress on post-DNAT traffic, you may also need `spec.network.apiServerEndpointIPs`. See <SiteLink docId="user-guide/openbaocluster/configuration/network">Network configuration</SiteLink>.

</Callout>

<Callout type="note" title="AppArmor on local clusters">

If kubelet rejects the Pods because AppArmor is unavailable, add:

```yaml
spec:
  workloadHardening:
    appArmorEnabled: false
```

</Callout>

## Step 5: Expose the passthrough path

<CommandBlock
  language="yaml"
  label="apply"
  title="Create the validated Traefik passthrough route"
  code={`apiVersion: traefik.io/v1alpha1
kind: IngressRouteTCP
metadata:
  name: bao-hardened
  namespace: <namespace>
spec:
  entryPoints:
    - websecure
  routes:
    - match: HostSNI(\`<external-host>\`)
      services:
        - name: <cluster-name>-public
          port: 8200
  tls:
    passthrough: true`}
>
  This lane uses a user-managed Traefik `IngressRouteTCP`, not `spec.gateway`. Keep the passthrough route separate from any shared terminating edge.
</CommandBlock>

## Verify the lane

<CommandBlock
  language="bash"
  label="verify"
  title="Check the cluster conditions"
  code={`kubectl -n <namespace> get openbaocluster <cluster-name> \\
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\\n"}{end}'`}
>
  The steady-state expectation is `Available=True`, `TLSReady=True`, `UserAccessBootstrap=True`, `ProductionReady=True`, and `OpenBaoInitialized=True`.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Confirm the cluster did not persist a root token Secret"
  code={`kubectl -n <namespace> get secret <cluster-name>-root-token`}
>
  This should return `NotFound`. A hardened self-init lane should not leave the root token stored as a Kubernetes Secret.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Exchange a Kubernetes JWT for an OpenBao admin token"
  code={`kubectl -n <namespace> port-forward svc/<cluster-name> 8200:8200
export VAULT_ADDR="https://127.0.0.1:8200"
JWT="$(kubectl -n <namespace> create token openbao-admin --audience openbao-internal --duration=1h)"

curl -sS -k \\
  -H 'Content-Type: application/json' \\
  -d "{\"role\":\"admin\",\"jwt\":\"\${JWT}\"}" \\
  "\${VAULT_ADDR%/}/v1/auth/jwt/login"`}
>
  The validated local path uses self-signed certificates, so the example uses `-k`. In a real environment with trusted certificates, remove that shortcut.
</CommandBlock>

<Callout type="note" title="What matters most in this lane">

The important exposure contract here is successful end-to-end passthrough plus the `trustedIngressPeers` rule. `GatewayIntegrationReady` is not the primary signal because the route is intentionally managed outside `spec.gateway`.

</Callout>

<NextActions
  title="After the lane is running"
  items={[
    {
      label: "Plan upgrades",
      description: "Move into the generic operating path once the validated lane is healthy.",
      docId: "user-guide/openbaocluster/operations/upgrades",
    },
    {
      label: "Configure backups",
      description: "Add snapshot coverage before you use the lane as a hardened rehearsal environment.",
      docId: "user-guide/openbaocluster/operations/backups",
    },
    {
      label: "k3d Hardened / ACME",
      description: "Compare this lane with the local hardened ACME lane when you want OpenBao-managed certificate issuance instead of external TLS Secrets.",
      docId: "user-guide/validated-deployments/architectures/local/k3d-hardened-transit-acme",
    },
  ]}
/>
