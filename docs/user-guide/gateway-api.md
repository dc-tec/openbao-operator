# Gateway API Support

## Prerequisites

- **Gateway API Implementation**: Installed (e.g., Envoy Gateway, Istio, Cilium, Traefik)
- **CRDs**: Gateway API CRDs installed
- **Gateway**: An existing `Gateway` resource configured

## Overview

The Operator supports [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/) as a modern alternative to Ingress for external access. Gateway API provides more expressive routing, better multi-tenancy support, and is more portable across implementations.

## Basic Gateway API Example

First, ensure you have a Gateway resource (typically managed by your platform team):

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: main-gateway
  namespace: gateway-system
spec:
  gatewayClassName: envoy
  listeners:
    - name: https
      protocol: HTTPS
      port: 443
      tls:
        mode: Terminate
        certificateRefs:
          - name: wildcard-cert
      allowedRoutes:
        namespaces:
          from: All
```

Then configure your OpenBaoCluster to use it:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: gateway-cluster
  namespace: security
spec:
  version: "2.4.0"
  image: "openbao/openbao:2.4.0"
  replicas: 3
  profile: Development
  tls:
    enabled: true
    rotationPeriod: "720h"
  storage:
    size: "10Gi"
  gateway:
    enabled: true
    gatewayRef:
      name: main-gateway
      namespace: gateway-system
    hostname: bao.example.com
    path: /
  deletionPolicy: Retain
```

A full sample using Gateway API (including TLS and a realistic Gateway reference) is provided in `config/samples/integrations/openbao_v1alpha1_openbaocluster_gateway.yaml`.

### 8.3 Gateway Configuration Options

| Field | Description | Required |
|-------|-------------|----------|
| `enabled` | Enable Gateway API support | Yes |
| `gatewayRef.name` | Name of the Gateway resource | Yes |
| `gatewayRef.namespace` | Namespace of the Gateway (defaults to cluster namespace) | No |
| `hostname` | Hostname for routing (added to TLS SANs) | Yes |
| `path` | Path prefix for routing (defaults to `/`) | No |
| `annotations` | Annotations for the HTTPRoute/TLSRoute | No |
| `tlsPassthrough` | Enable TLS passthrough using TLSRoute (defaults to `false`) | No |

### 8.4 What the Operator Creates

When Gateway API is enabled, the Operator creates either an `HTTPRoute` (default) or a `TLSRoute` (when `tlsPassthrough: true`):

**HTTPRoute (TLS Termination - Default):**

When `spec.gateway.tlsPassthrough` is `false` or omitted, the Operator creates an `HTTPRoute`:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: gateway-cluster-httproute
  namespace: security
spec:
  parentRefs:
    - name: main-gateway
      namespace: gateway-system
  hostnames:
    - bao.example.com
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: gateway-cluster-public
          port: 8200
```

Verify the HTTPRoute was created:

```sh
kubectl -n security get httproutes
```

**TLSRoute (TLS Passthrough - Optional):**

When `spec.gateway.tlsPassthrough` is `true`, the Operator creates a `TLSRoute` instead of an `HTTPRoute`:

```yaml
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: TLSRoute
metadata:
  name: gateway-cluster-tlsroute
  namespace: security
spec:
  parentRefs:
    - name: main-gateway
      namespace: gateway-system
  hostnames:
    - bao.example.com
  rules:
    - backendRefs:
        - name: gateway-cluster-public
          port: 8200
```

**Key Differences:**

- **HTTPRoute**: Gateway terminates TLS, decrypts traffic, and forwards HTTP to OpenBao. Requires `BackendTLSPolicy` for Gateway→OpenBao HTTPS.
- **TLSRoute**: Gateway passes encrypted TLS traffic through based on SNI. OpenBao terminates TLS directly. No `BackendTLSPolicy` needed.

**Gateway Configuration for TLS Passthrough:**

When using TLS passthrough, the Gateway listener must be configured with `protocol: TLS` and `mode: Passthrough`:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: main-gateway
  namespace: gateway-system
spec:
  gatewayClassName: traefik
  listeners:
    - name: tls-passthrough
      port: 443
      protocol: TLS
      hostname: "*.example.com"
      tls:
        mode: Passthrough
```

**When to Use TLS Passthrough:**

- End-to-end encryption requirements (Gateway never decrypts traffic)
- Client certificate authentication at OpenBao
- Compliance requirements for no Gateway decryption
- Performance optimization (single TLS handshake)

**Note:** TLSRoute is in the Experimental channel of Gateway API. Ensure your Gateway implementation supports it.

### 8.5 Gateway API vs Ingress

| Feature | Ingress | Gateway API |
|---------|---------|-------------|
| Configuration | `spec.ingress` | `spec.gateway` |
| Resource Created | `Ingress` | `HTTPRoute` |
| TLS Handling | Per-Ingress | Per-Gateway (shared) |
| Multi-tenancy | Limited | First-class support |
| Portability | Controller-specific annotations | Standardized API |

You can use either Ingress or Gateway API, but not both simultaneously on the same cluster.

### 8.6 End-to-End TLS with Gateway API

**HTTPRoute with TLS Termination:**

For end-to-end TLS when using HTTPRoute (TLS termination at Gateway), the operator automatically creates a `BackendTLSPolicy` that configures the Gateway to use HTTPS when communicating with the OpenBao backend service and validates the backend certificate using the cluster's CA certificate.

The operator automatically creates:

- A CA Secret named `<cluster-name>-tls-ca` containing the CA certificate.
- A ConfigMap with the same name containing the CA certificate in the `ca.crt` key (required for implementations like Traefik that only support `ConfigMap` references).
- A `BackendTLSPolicy` named `<cluster-name>-backend-tls-policy` that references the CA ConfigMap and configures TLS validation.

**Automatic BackendTLSPolicy Creation:**

By default, when `spec.gateway.enabled = true` and TLS is enabled (`spec.tls.enabled = true`), the operator automatically creates the `BackendTLSPolicy`. You can disable this behavior by setting `spec.gateway.backendTLS.enabled = false`:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: my-cluster
spec:
  profile: Development
  gateway:
    enabled: true
    gatewayRef:
      name: traefik-gateway
    hostname: bao.example.com
    backendTLS:
      enabled: false  # Disable automatic BackendTLSPolicy creation
```

**Custom Hostname:**

You can specify a custom hostname for certificate validation by setting `spec.gateway.backendTLS.hostname`. If not specified, the operator defaults to the Service DNS name (`<service-name>.<namespace>.svc`):

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: my-cluster
spec:
  profile: Development
  gateway:
    enabled: true
    gatewayRef:
      name: traefik-gateway
    hostname: bao.example.com
    backendTLS:
      hostname: custom-hostname.example.com  # Custom hostname for validation
```

**Example Generated BackendTLSPolicy:**

The operator creates a `BackendTLSPolicy` similar to:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: BackendTLSPolicy
metadata:
  name: my-cluster-backend-tls-policy
  namespace: default
spec:
  targetRefs:
    - group: ""
      kind: Service
      name: my-cluster-public
  validation:
    caCertificateRefs:
      - group: ""
        kind: ConfigMap
        name: my-cluster-tls-ca
    hostname: my-cluster-public.default.svc
```

This configuration tells the Gateway implementation to:

- Use HTTPS when talking to the OpenBao backend Service.
- Validate the backend certificate using the CA published by the operator.

**Note:** `BackendTLSPolicy` availability (including the exact `apiVersion`) depends on your Gateway API implementation and version. The operator gracefully handles cases where the `BackendTLSPolicy` CRD is not installed.

**TLSRoute with TLS Passthrough:**

When using TLSRoute (`spec.gateway.tlsPassthrough: true`), `BackendTLSPolicy` is not needed because the Gateway does not decrypt traffic. The Gateway simply routes encrypted TLS traffic based on SNI, and OpenBao terminates TLS directly. This provides true end-to-end encryption where the Gateway never sees decrypted content.
