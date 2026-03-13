---
description: Step-by-step recipe for a Hardened OpenBao cluster using Transit auto-unseal, self-init, externally managed TLS Secrets, and validated local passthrough access.
---

# Hardened Transit with External TLS

This recipe deploys a production-style `OpenBaoCluster` with:

- `spec.profile: Hardened`
- Transit auto-unseal
- `spec.tls.mode: External`
- `spec.selfInit.enabled: true`
- JWT login for a human admin `ServiceAccount`

!!! success "Validated by E2E"
    This recipe follows the Hardened profile lifecycle covered by the in-repo E2E suite, especially the `Hardened profile (External TLS + Transit auto-unseal + SelfInit)` suite. That suite validates tenant onboarding, external TLS Secrets, Transit auto-unseal, self-init, and rolling or blue/green upgrades.

## Prerequisites

- OpenBao Operator is installed in multi-tenant mode with admission policies enabled.
- cert-manager is installed.
- A Transit-capable OpenBao instance is reachable from the cluster.
- You have a Transit token with `update` access to:
  - `transit/encrypt/<key-name>`
  - `transit/decrypt/<key-name>`
- You know the namespace used by your ingress controller if you plan to expose the cluster externally.

## Inputs

Replace these values before applying the manifests:

| Placeholder | Example | Purpose |
| :--- | :--- | :--- |
| `<namespace>` | `openbaocluster-hardened` | Tenant namespace for the cluster |
| `<cluster-name>` | `openbaocluster-hardened` | `OpenBaoCluster` name |
| `<openbao-version>` | `2.5.0` | OpenBao version |
| `<transit-address>` | `https://infra-bao.openbao-infra.svc:8200` | Transit provider URL |
| `<transit-key>` | `openbao-unseal` | Transit key name |
| `<external-host>` | `bao-hardened.example.com` | External DNS name for clients |
| `<ingress-namespace>` | `default` | Namespace of the ingress controller that forwards traffic to OpenBao |
| `<transit-namespace>` | `openbao-infra` | Namespace hosting the Transit provider |

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

## Step 2: Create the Transit credential Secret

Create the Secret referenced by `spec.unseal.credentialsSecretRef`:

```bash
kubectl -n <namespace> create secret generic infra-bao-token \
  --from-literal=token='<transit-token>' \
  --from-file=ca.crt=/path/to/infra-bao-ca.crt
```

!!! note "Expected Secret keys"
    For the validated path, the Secret contains:

    - `token`: Transit token used as `VAULT_TOKEN`
    - `ca.crt`: CA bundle used as `VAULT_CACERT`

## Step 3: Create the External TLS Secrets

For the validated local path, use cert-manager to create the Secrets expected by `tls.mode: External`.

```yaml
apiVersion: cert-manager.io/v1
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
    name: <cluster-name>-ca-issuer
```

Wait for the certificates to become ready:

```bash
kubectl -n <namespace> wait certificate/<cluster-name>-tls-ca --for=condition=Ready --timeout=5m
kubectl -n <namespace> wait certificate/<cluster-name>-tls-server --for=condition=Ready --timeout=5m
```

!!! note "Corporate PKI"
    If you already use cert-manager with a corporate issuer, replace the self-signed Issuer objects and keep the Secret names:

    - `<cluster-name>-tls-ca`
    - `<cluster-name>-tls-server`

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
    mode: External

  configuration:
    logLevel: "info"
    ui: true
    logging:
      format: "json"

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

!!! warning "API server endpoint IPs"
    If your CNI enforces egress on post-DNAT traffic, you may also need `spec.network.apiServerEndpointIPs`. See [Network Configuration](../configuration/network.md).

## Step 5: Expose the cluster (validated local path)

For the validated local path, Traefik exposes the OpenBao public Service through TCP passthrough:

```yaml
apiVersion: traefik.io/v1alpha1
kind: IngressRouteTCP
metadata:
  name: bao-hardened
  namespace: <namespace>
spec:
  entryPoints:
    - websecure
  routes:
    - match: HostSNI(`<external-host>`)
      services:
        - name: <cluster-name>-public
          port: 8200
  tls:
    passthrough: true
```

If you use Gateway API instead, keep the `OpenBaoCluster` manifest and replace this Traefik resource with your own passthrough route or terminating Gateway configuration.

## Operations

### Verify the cluster is ready

Check the status conditions:

```bash
kubectl -n <namespace> get openbaocluster <cluster-name> \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{" reason="}{.reason}{"\n"}{end}'
```

The steady-state expectation is:

- `Available=True`
- `TLSReady=True`
- `UserAccessBootstrap=True`
- `ProductionReady=True`
- `OpenBaoInitialized=True`
- `APIServerNetworkReady=True` or `Unknown` with reason `APIServerEndpointIPsRecommended`

Confirm that no root token Secret was created:

```bash
kubectl -n <namespace> get secret <cluster-name>-root-token
```

This should return `NotFound`.

!!! note "User-managed passthrough"
    This recipe uses a user-managed Traefik TCP passthrough route, not `spec.gateway`. The important exposure contract here is the `trustedIngressPeers` rule plus successful end-to-end traffic, not `GatewayIntegrationReady`.

### Verify JWT admin login

If you exposed the cluster externally, set `VAULT_ADDR` to the external hostname. Otherwise, port-forward the Service first.

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

!!! note "TLS verification in local validation"
    The validated local path uses self-signed certificates, so the example uses `-k`. In production, use trusted certificates and remove `-k`.

## Common Failures

- `TLSReady=False` with `TLSSecretMissing`: the external TLS Secrets are missing or not Ready yet.
- `UserAccessBootstrap=Unknown`: `spec.selfInit.requests` did not give the operator a recognizable human login path.
- `ProductionReady=False` with `RootTokenStored`: `selfInit.enabled` is not set to `true`.
- `ProductionReady=False` with `OperatorManagedTLS`: `tls.mode` is not `External` or `ACME`.
- Transit connection failures: verify the Secret keys, Transit token policy, and the CA bundle used by `tlsCACert`.

## See Also

- [Security Profiles](../configuration/security-profiles.md)
- [Self-Initialization](../configuration/self-init.md)
- [Status Conditions and Events](../../../reference/status-and-events.md)
