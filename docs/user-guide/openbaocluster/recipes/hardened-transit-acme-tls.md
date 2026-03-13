---
description: Step-by-step recipe for a Hardened OpenBao cluster using Transit auto-unseal, an internal ACME CA, self-init, and validated local TLS passthrough.
---

# Hardened Transit with ACME TLS

This recipe deploys a production-style `OpenBaoCluster` with:

- `spec.profile: Hardened`
- Transit auto-unseal
- `spec.tls.mode: ACME`
- `spec.selfInit.enabled: true`
- JWT login for a human admin `ServiceAccount`

!!! success "Validated by E2E"
    This recipe follows the ACME TLS lifecycle covered by the in-repo E2E suite, especially the `ACME TLS (OpenBao native ACME client)` suite. That suite validates private ACME CA trust material, Transit auto-unseal, ACME readiness, and certificate verification.

## Prerequisites

- OpenBao Operator is installed in multi-tenant mode with admission policies enabled.
- A Transit-capable OpenBao instance is reachable from the cluster.
- The same external OpenBao instance also exposes an ACME directory endpoint.
- You have a Secret payload containing:
  - `token`
  - `ca.crt`
  - `pki-ca.crt`
- Your external hostname resolves to the ingress controller from inside the cluster.
- Your external exposure layer supports TLS passthrough on port `443`.

## Inputs

Replace these values before applying the manifests:

| Placeholder | Example | Purpose |
| :--- | :--- | :--- |
| `<namespace>` | `openbaocluster-acme` | Tenant namespace for the cluster |
| `<cluster-name>` | `openbaocluster-acme` | `OpenBaoCluster` name |
| `<openbao-version>` | `2.5.0` | OpenBao version |
| `<transit-address>` | `https://infra-bao.openbao-infra.svc:8200` | Transit provider URL |
| `<acme-directory-url>` | `https://infra-bao.openbao-infra.svc:8200/v1/pki/acme/directory` | ACME directory URL |
| `<transit-key>` | `openbao-unseal` | Transit key name |
| `<external-host>` | `bao-acme.example.com` | External DNS name for clients and ACME validation |
| `<ingress-namespace>` | `default` | Namespace of the ingress controller that forwards traffic to OpenBao |
| `<transit-namespace>` | `openbao-infra` | Namespace hosting the Transit and ACME provider |

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
  namespace: openbao-operator-system
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
kubectl -n openbao-operator-system describe openbaotenant <cluster-name>-tenant
```

The steady-state expectation is `Provisioned=True` on the `OpenBaoTenant`.

## Step 2: Create the Transit and ACME trust Secret

Create the Secret referenced by `spec.unseal.credentialsSecretRef`:

```bash
kubectl -n <namespace> create secret generic infra-bao-token \
  --from-literal=token='<transit-token>' \
  --from-file=ca.crt=/path/to/infra-bao-ca.crt \
  --from-file=pki-ca.crt=/path/to/infra-bao-pki-ca.crt
```

!!! note "Expected Secret keys"
    For the validated path, the Secret contains:

    - `token`: Transit token used as `VAULT_TOKEN`
    - `ca.crt`: CA bundle for the Transit and ACME directory endpoint
    - `pki-ca.crt`: CA bundle that signs the ACME-issued leaf certificates

## Step 3: Expose the ACME challenge Service with passthrough

The validated local path uses Traefik TCP passthrough so OpenBao can terminate TLS itself:

```yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRouteTCP
metadata:
  name: bao-acme
  namespace: <namespace>
spec:
  entryPoints:
    - websecure
  routes:
    - match: HostSNI(`<external-host>`)
      services:
        - name: <cluster-name>-acme
          port: 443
  tls:
    passthrough: true
```

!!! warning "Passthrough is required"
    `tls.mode: ACME` requires TLS passthrough. If your Gateway or ingress controller terminates TLS first, OpenBao cannot complete ACME challenges.

## Step 4: Apply the OpenBaoCluster

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
      name: infra-bao-token
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
            port: 8200
```

!!! note "Internal `.svc` domain"
    The first entry in `spec.tls.acme.domains` is intentional. For the validated local path, the internal `.svc` hostname gives OpenBao a stable SNI and Raft join target while the external hostname remains present in the certificate SANs.

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
- `UserAccessBootstrap=True`
- `ProductionReady=True`
- `OpenBaoInitialized=True`
- `OpenBaoSealed=False`
- `APIServerNetworkReady=True` or `Unknown` with reason `APIServerEndpointIPsRecommended`

Verify that the dedicated ACME Service exists:

```bash
kubectl -n <namespace> get svc <cluster-name>-acme
```

Verify that no external TLS Secret was required:

```bash
kubectl -n <namespace> get secret <cluster-name>-tls-server
```

This should return `NotFound`.

!!! note "User-managed passthrough"
    This recipe uses a user-managed passthrough route instead of `spec.gateway`, so `GatewayIntegrationReady` is not the primary checkpoint here. For ACME, the important operator-owned conditions are `ACMEIntegrationReady` and `ACMECacheReady`.

### Verify JWT admin login

Port-forward the main Service for direct access:

```bash
kubectl -n <namespace> port-forward svc/<cluster-name> 8200:8200
export VAULT_ADDR="https://127.0.0.1:8200"
```

Create a Kubernetes token for the admin `ServiceAccount` and exchange it for an OpenBao token:

```bash
JWT="$(kubectl -n <namespace> create token openbao-admin --audience openbao-internal --duration=1h)"

curl -sS -k \
  -H 'Content-Type: application/json' \
  -d "{\"role\":\"admin\",\"jwt\":\"${JWT}\"}" \
  "${VAULT_ADDR%/}/v1/auth/jwt/login"
```

## Common Failures

- `Degraded=True` with `ACMEGatewayNotConfiguredForPassthrough`: your exposure layer is terminating TLS instead of passing it through.
- `Degraded=True` with `ACMEDomainNotResolvable`: the configured hostname does not resolve from inside the cluster.
- `UserAccessBootstrap=Unknown`: `spec.selfInit.requests` did not give the operator a recognizable human login path.
- Transit connection failures: verify the Secret keys, Transit token policy, and the CA bundle used by `tlsCACert`.
- Raft join or probe verification errors with a private ACME CA: verify that `pki-ca.crt` is present in the same Secret and mounted alongside `acmeCARoot`.

## See Also

- [Gateway API Support](../configuration/gateway-api.md)
- [External Access](../configuration/external-access.md)
- [Troubleshooting](../operations/troubleshooting.md)
