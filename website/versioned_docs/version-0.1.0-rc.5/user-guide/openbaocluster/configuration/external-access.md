# External Access

OpenBao clusters can be exposed using **Gateway API** (Recommended), **Ingress**, or standard **LoadBalancer** services.

## Access Methods

<Tabs groupId="gateway-api-recommended-ingress-service-l4">

<TabItem value="gateway-api-recommended" label="Gateway API (Recommended)">

The Operator provides first-class support for [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/), offering advanced routing, portability, and cleaner multi-tenancy.
For OpenBao, prefer TLS passthrough so OpenBao remains the TLS endpoint.

<Callout type="tip" title="Full Guide">

See the [Gateway API Guide](gateway-api.md) for complete configuration details, including TLS Passthrough and backend policies.

</Callout>

```yaml
spec:
  gateway:
    enabled: true
    tlsPassthrough: true
    hostname: bao.example.com
    gatewayRef:
      name: main-gateway
      namespace: gateway-system
```

</TabItem>

<TabItem value="ingress" label="Ingress">

Standard Kubernetes Ingress support.

```yaml
spec:
  ingress:
    enabled: true
    host: "bao.example.com"
    annotations:
      nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"
```

<Callout type="note" title="Traefik v3">

Traefik v3 requires a `ServersTransport` to trust the internal CA. See the [Traefik v3 Configuration](#traefik-v3-configuration) section below.

</Callout>

</TabItem>

<TabItem value="service-l4" label="Service (L4)">

Expose the cluster directly via a LoadBalancer or NodePort service.

```yaml
spec:
  service:
    type: LoadBalancer
    annotations:
      service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
```

</TabItem>

</Tabs>

## TLS Configuration

Secure your cluster using one of the following TLS modes.

<Tabs groupId="acme-let-s-encrypt-external-pki-operator-managed">

<TabItem value="acme-let-s-encrypt" label="ACME (Let's Encrypt)">

**Zero-Trust:** OpenBao acts as a native ACME client (e.g., Let's Encrypt), managing its own certificates without mounting Secrets.

```yaml
spec:
  tls:
    enabled: true
    mode: ACME
    acme:
      directoryURL: "https://acme-v02.api.letsencrypt.org/directory"
      # Prefer domains (list) for multi-SAN certificates
      domains:
        - "bao.example.com"
      email: "admin@example.com"
```

<Callout type="note" title="HA (Raft) + private ACME CA">

When using a private ACME CA (for example, an in-cluster PKI), peers must trust the **PKI CA**
that signs the issued leaf certificate. If `spec.configuration.acmeCARoot` is set to trust the
ACME directory server, place a `pki-ca.crt` file alongside it in the same volume; the operator
uses it for Raft `retry_join` and probe verification.

</Callout>

</TabItem>

<TabItem value="external-pki" label="External PKI">

**BYO-Cert:** Integrate with `cert-manager` or corporate PKI. You provide the Secrets; the Operator uses them.

```yaml
spec:
  tls:
    enabled: true
    mode: External
```

**Requirements:**
- Secret `<name>-tls-ca`: Keys `ca.crt` (optional `ca.key`)
- Secret `<name>-tls-server`: Keys `tls.crt`, `tls.key`, `ca.crt`

</TabItem>

<TabItem value="operator-managed" label="Operator Managed">

**Default:** The Operator manages an internal CA and rotates certificates automatically. Use this for development or internal evaluation, not for Hardened production.

```yaml
spec:
  tls:
    enabled: true
    # mode defaults to OperatorManaged
```

</TabItem>

</Tabs>

## Advanced Configuration

### Traefik v3 Configuration

Traefik v3 enforces potential CA validation for backends. The Operator creates a Secret named `<cluster>-tls-ca` which Traefik can reference directly in a `ServersTransport`.

```yaml
apiVersion: traefik.io/v1alpha1
kind: ServersTransport
metadata:
  name: openbao-tls-transport
spec:
  rootCAsSecrets:
    - my-cluster-tls-ca
```

## Official OpenBao Documentation

- [TCP Listener Configuration](https://openbao.org/docs/configuration/listener/tcp/)
- [ACME TLS Listener Configuration](https://openbao.org/docs/configuration/listener/tcp/#acme-parameters)

