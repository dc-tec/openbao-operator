---
title: k3d Hardened / ACME Recipe
hide_title: true
pageType: task
journey: validated-deployments
description: Reproduce the validated local hardened ACME lane with Transit auto-unseal, a private ACME issuer, self-init, and user-managed TLS passthrough.
---

<PageHero
  eyebrow="Validated Deployments / Local Baselines / k3d Hardened / ACME"
  title="Reproduce the validated hardened ACME lane while keeping OpenBao as the TLS endpoint all the way through the local edge."
  lede="This recipe stands up the local hardened ACME baseline with tenant onboarding, a shared trust-services dependency for Transit and ACME, an internal ACME CA, and a user-managed passthrough route that preserves `tls-alpn-01` behavior."
  actions={[
    {label: "Open reference architecture", docId: "user-guide/validated-deployments/architectures/local/k3d-hardened-transit-acme", variant: "primary"},
    {label: "Open troubleshooting", docId: "user-guide/openbaocluster/operations/troubleshooting", variant: "secondary"},
  ]}
>
  <Checklist
    title="This recipe should leave you with"
    items={[
      "an onboarded tenant namespace and admin ServiceAccount",
      "a trust Secret that carries both the Transit CA bundle and the private ACME issuer trust root",
      "a hardened cluster that self-initializes, unseals with Transit, and serves ACME traffic through passthrough",
      "conditions and services that prove ACME is working before you move to a public cloud lane",
    ]}
  />
</PageHero>

<Callout type="success" title="Validated lane">

This recipe follows the local ACME lifecycle covered by the in-repo ACME suite and the project validation environment. The tested path covers private ACME trust material, Transit auto-unseal, ACME readiness, and human admin JWT access.

</Callout>

<Callout type="note" title="Use the main docs for generic behavior">

Use <SiteLink docId="user-guide/openbaocluster/configuration/external-access">external access</SiteLink>, <SiteLink docId="user-guide/openbaocluster/configuration/network">network configuration</SiteLink>, and <SiteLink docId="security/workload/tls">TLS and workload identity</SiteLink> for the product-wide explanation. This recipe captures the exact validated local lane.

</Callout>

<DecisionTable
  title="What this lane assumes"
  columns={["Assumption", "Why it exists", "What breaks if it is wrong"]}
  rows={[
    {
      cells: [
        "Multi-tenant operator install with admission enabled",
        "The validated path starts from the standard tenant-onboarding flow.",
        "Namespace provisioning and hardened policy enforcement will drift from the tested lane.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "The external trust-services endpoint exposes both Transit and an ACME directory",
        "One dependency owns both the seal root and the private certificate issuance path.",
        "You will not reproduce the lane if Transit and ACME trust material come from unrelated systems.",
      ],
    },
    {
      cells: [
        "The external hostname resolves back to the passthrough edge",
        "`tls-alpn-01` only works when the validator reaches the hostname that OpenBao serves.",
        "ACME will fail even though the cluster itself is healthy and the route object exists.",
      ],
    },
    {
      cells: [
        "The ingress layer supports TLS passthrough on port `443`",
        "OpenBao must terminate TLS itself in this lane.",
        "Any edge termination in front of OpenBao breaks the ACME contract immediately.",
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
    {cells: ["`<namespace>`", "`openbaocluster-acme`", "Tenant namespace for the cluster."]},
    {cells: ["`<cluster-name>`", "`openbaocluster-acme`", "`OpenBaoCluster` name."]},
    {cells: ["`<openbao-version>`", "`2.5.0`", "OpenBao version."]},
    {cells: ["`<transit-address>`", "`https://trust-services.openbao-infra.svc:8200`", "Transit provider URL."]},
    {cells: ["`<acme-directory-url>`", "`https://trust-services.openbao-infra.svc:8200/v1/pki/acme/directory`", "ACME directory URL."]},
    {cells: ["`<transit-key>`", "`openbao-unseal`", "Transit key name."]},
    {cells: ["`<external-host>`", "`bao-acme.example.com`", "External hostname used for clients and ACME validation."]},
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

## Step 2: Create the Transit and ACME trust Secret

<CommandBlock
  language="bash"
  label="apply"
  title="Create the Secret used by Transit and the private ACME issuer"
  code={`kubectl -n <namespace> create secret generic trust-services-token \\
  --from-literal=token='<transit-token>' \\
  --from-file=ca.crt=/path/to/trust-services-ca.crt \\
  --from-file=pki-ca.crt=/path/to/acme-issuer-ca.crt`}
>
  The validated path expects `token`, `ca.crt`, and `pki-ca.crt` in the same Secret so Transit and ACME trust material stay aligned.
</CommandBlock>

## Step 3: Expose the ACME challenge Service with passthrough

<CommandBlock
  language="yaml"
  label="apply"
  title="Create the user-managed passthrough route"
  code={`apiVersion: traefik.io/v1alpha1
kind: IngressRouteTCP
metadata:
  name: bao-acme
  namespace: <namespace>
spec:
  entryPoints:
    - websecure
  routes:
    - match: HostSNI(\`<external-host>\`)
      services:
        - name: <cluster-name>-acme
          port: 443
  tls:
    passthrough: true`}
>
  `tls.mode: ACME` requires passthrough. If the edge terminates TLS first, OpenBao cannot complete ACME challenges.
</CommandBlock>

## Step 4: Apply the hardened ACME cluster

<CommandBlock
  language="yaml"
  label="apply"
  title="Apply the validated hardened ACME manifest"
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
    mode: ACME
    acme:
      directoryURL: "<acme-directory-url>"
      domains:
        - "<cluster-name>-acme.<namespace>.svc"
        - "<external-host>"
      email: "admin@example.invalid"

  configuration:
    logLevel: "info"
    ui: true
    logging:
      format: "json"
    acmeCARoot: "/etc/bao/seal-creds/ca.crt"

  unseal:
    type: transit
    credentialsSecretRef:
      name: trust-services-token
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
    strategy: RollingUpdate`}
/>

<Callout type="note" title="Internal `.svc` domain">

The internal `.svc` hostname in `spec.tls.acme.domains` gives the local lane a stable service name for certificate issuance and cluster-internal joins while the external hostname stays present in the SAN set.

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
  The steady-state expectation is `Available=True`, `ACMEIntegrationReady=True`, `ACMECacheReady=True`, `UserAccessBootstrap=True`, `ProductionReady=True`, `OpenBaoInitialized=True`, and `OpenBaoSealed=False`.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Verify that the ACME Service exists and no external TLS Secret is required"
  code={`kubectl -n <namespace> get svc <cluster-name>-acme
kubectl -n <namespace> get secret <cluster-name>-tls-server`}
>
  The service should exist. The Secret lookup should return `NotFound`, because OpenBao manages the leaf certificate itself in this lane.
</CommandBlock>

<CommandBlock
  language="bash"
  label="verify"
  title="Verify JWT admin login"
  code={`kubectl -n <namespace> port-forward svc/<cluster-name> 8200:8200
export VAULT_ADDR="https://127.0.0.1:8200"
JWT="$(kubectl -n <namespace> create token openbao-admin --audience openbao-internal --duration=1h)"

curl -sS -k \\
  -H 'Content-Type: application/json' \\
  -d "{\\"role\\":\\"admin\\",\\"jwt\\":\\"\${JWT}\\"}" \\
  \${VAULT_ADDR%/}/v1/auth/jwt/login`}
/>

<NextActions
  title="Keep moving"
  items={[
    {
      label: "Reference architecture",
      description: "Review the lane summary, topology, and invariants behind this local ACME path.",
      docId: "user-guide/validated-deployments/architectures/local/k3d-hardened-transit-acme",
    },
    {
      label: "Troubleshoot the cluster",
      description: "Use the incident routing page if ACME domain reachability or certificate readiness does not settle.",
      docId: "user-guide/openbaocluster/operations/troubleshooting",
    },
    {
      label: "EKS Hardened",
      description: "Move to the cloud baseline when you need public ACME, KMS auto-unseal, and a dedicated passthrough Gateway.",
      docId: "user-guide/validated-deployments/architectures/cloud/amazon-eks-hardened-awskms-acme",
    },
  ]}
/>
