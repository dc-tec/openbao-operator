# External Access

## Prerequisites

- **Ingress Controller**: Installed (e.g., NGINX, Traefik, AWS Load Balancer Controller)
- **DNS**: Domain name configured (e.g., `bao.example.com`)

## Configuration

To expose OpenBao externally, configure `spec.service` and/or `spec.ingress`:

```yaml
spec:
  service:
    type: LoadBalancer
    annotations:
      service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
  ingress:
    enabled: true
    host: "bao.example.com"
    path: "/"
    annotations:
      nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"
```

With this configuration:

- The Operator ensures `dev-cluster-public` exists as an external Service.
- An `Ingress` named `dev-cluster` routes `https://bao.example.com/` to `dev-cluster-public`.
- The TLS Secret for the Ingress defaults to `dev-cluster-tls-server` unless `tlsSecretName` is set.
- The `bao.example.com` host is automatically added to the server certificate SANs.
- The external service DNS name (`{cluster-name}-public.{namespace}.svc`) is automatically added to the server certificate SANs when service or ingress is configured.

Always validate connectivity through your ingress or load balancer after the cluster reports `Available=True`.

### 6.1 Traefik v3 Configuration

When using Traefik v3 with TLS termination at the ingress and HTTPS backend communication, you need to configure Traefik to trust the OpenBaoCluster CA certificate:

1. **Create a ServersTransport** resource that references the existing CA secret (see `config/samples/traefik-servers-transport.yaml`):

   ```yaml
   apiVersion: traefik.io/v1alpha1
   kind: ServersTransport
   metadata:
     name: openbao-tls-transport
     namespace: <your-namespace>
   spec:
     rootCAsSecrets:
       - <cluster-name>-tls-ca  # e.g., openbaocluster-external-tls-ca
   ```

   The operator automatically creates the CA secret named `<cluster-name>-tls-ca`. Traefik will read the `ca.crt` key from this secret directly - no mounting required!

2. **Reference the transport** in your OpenBaoCluster service annotations (already included in the sample):

   ```yaml
   service:
     annotations:
       traefik.ingress.kubernetes.io/service.serversscheme: https
       traefik.ingress.kubernetes.io/service.serverstransport: openbao-tls-transport
   ```

   Note: If the ServersTransport is in the same namespace as the Service, use just the name. For cross-namespace references, use `<namespace>-<name>@kubernetescrd` and ensure `allowCrossNamespace` is enabled in Traefik.

This ensures Traefik validates the OpenBao backend certificate using the cluster's CA, maintaining secure end-to-end TLS.

### 6.2 External TLS Provider (cert-manager)

For production environments that require integration with corporate PKI or cert-manager, you can configure the operator to use externally-managed TLS certificates.

Set `spec.tls.mode` to `External`:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
  namespace: security
spec:
  version: "2.4.4"
  image: "openbao/openbao:2.4.4"
  replicas: 3
  tls:
    enabled: true
    mode: External  # Use external certificate management
  storage:
    size: "10Gi"
```

**Important:** When using External mode, you must create the TLS Secrets before the operator can proceed. The operator expects:

- `<cluster-name>-tls-ca` - CA certificate Secret with keys `ca.crt` and optionally `ca.key`
- `<cluster-name>-tls-server` - Server certificate Secret with keys `tls.crt`, `tls.key`, and `ca.crt`

### 6.3 ACME TLS (Native OpenBao ACME Client)

For zero-trust architectures and environments where you want OpenBao to manage certificates automatically via the ACME protocol (e.g., Let's Encrypt), you can configure ACME mode.

Set `spec.tls.mode` to `ACME`:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
  namespace: security
spec:
  version: "2.4.4"
  image: "openbao/openbao:2.4.4"
  replicas: 3
  profile: Hardened  # ACME mode is compatible with Hardened profile
  tls:
    enabled: true
    mode: ACME
    acme:
      directoryURL: "https://acme-v02.api.letsencrypt.org/directory"
      domain: "openbao.example.com"
      email: "admin@example.com"  # Optional: for CA notifications
  storage:
    size: "10Gi"
  selfInit:
    enabled: true
  unseal:
    type: awskms
    awskms:
      region: us-east-1
      kmsKeyID: alias/openbao-unseal
```

**Key Characteristics of ACME Mode:**

- **No Secrets:** No TLS Secrets are created or mounted. Certificates are managed entirely by OpenBao.
- **No Wrapper Needed:** No TLS reload wrapper is needed, and `ShareProcessNamespace` is disabled for better container isolation.
- **Automatic Rotation:** OpenBao handles certificate acquisition and rotation automatically via the ACME protocol.
- **Zero Trust:** The operator never possesses private keys, making this ideal for zero-trust architectures.
- **Hardened Profile Compatible:** ACME mode is accepted by the Hardened security profile.

**ACME Configuration Parameters:**

- `directoryURL` (required): The ACME directory URL (e.g., `https://acme-v02.api.letsencrypt.org/directory` for Let's Encrypt production)
- `domain` (required): The domain name for which to obtain the certificate
- `email` (optional): Email address for CA notifications about certificate expiration

**Note:** For internal networks, you may need to use DNS challenges instead of HTTP challenges. See [OpenBao's ACME documentation](https://openbao.org/docs/configuration/listener/tcp/#acme-parameters) for advanced configuration options.

**Example with cert-manager:**

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: prod-cluster-server
  namespace: security
spec:
  secretName: prod-cluster-tls-server  # Must match operator expectation
  dnsNames:
    - prod-cluster.security.svc
    - *.prod-cluster.security.svc
    - prod-cluster-public.security.svc
  issuerRef:
    name: corporate-ca-issuer
    kind: ClusterIssuer
```

The operator will:

- Wait for the Secrets to be created (no error if missing)
- Monitor certificate changes and trigger hot-reloads when cert-manager rotates certificates
- Not attempt to generate or rotate certificates itself

**Note:** The `rotationPeriod` field is ignored in External mode, as certificate rotation is handled by the external provider.
