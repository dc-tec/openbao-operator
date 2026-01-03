# External Access

OpenBao clusters can be exposed using **Gateway API** (Recommended), **Ingress**, or standard **LoadBalancer** services.

## Access Methods

=== "Gateway API (Recommended)"
    The Operator provides first-class support for [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/), offering advanced routing, portability, and cleaner multi-tenancy.

    !!! tip "Full Guide"
        See the [Gateway API Guide](gateway-api.md) for complete configuration details, including TLS Passthrough and backend policies.

    ```yaml
    spec:
      gateway:
        enabled: true
        hostname: bao.example.com
        gatewayRef:
          name: main-gateway
          namespace: gateway-system
    ```

=== "Ingress"
    Standard Kubernetes Ingress support.

    ```yaml
    spec:
      ingress:
        enabled: true
        host: "bao.example.com"
        annotations:
          nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"
    ```

    !!! info "Traefik v3"
        Traefik v3 requires a `ServersTransport` to trust the internal CA. See the [Traefik v3 Configuration](#traefik-v3-configuration) section below.

=== "Service (L4)"
    Expose the cluster directly via a LoadBalancer or NodePort service.

    ```yaml
    spec:
      service:
        type: LoadBalancer
        annotations:
          service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
    ```

## TLS Configuration

Secure your cluster using one of the following TLS modes.

=== "ACME (Let's Encrypt)"
    **Zero-Trust:** OpenBao acts as a native ACME client (e.g., Let's Encrypt), managing its own certificates without mounting Secrets.

    ```yaml
    spec:
      tls:
        enabled: true
        mode: ACME
        acme:
          directoryURL: "https://acme-v02.api.letsencrypt.org/directory"
          domain: "bao.example.com"
          email: "admin@example.com"
    ```

=== "External PKI"
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

=== "Operator Managed"
    **Default:** The Operator manages an internal CA and rotates certificates automatically. Good for internal use or testing.

    ```yaml
    spec:
      tls:
        enabled: true
        # mode defaults to OperatorManaged
    ```

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
