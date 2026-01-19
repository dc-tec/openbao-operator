# Recipe: Hardened + Transit + External TLS

## Introduction

This recipe shows a production-style configuration using:
- `spec.profile: Hardened`
- Transit auto-unseal
- `tls.mode: External` (cert-manager/corporate PKI/CSI-managed secrets)

## Prerequisites

- An external certificate issuer (cert-manager or equivalent) that produces the required Secrets.
- A Transit-capable OpenBao instance reachable from the cluster (for unseal).

## Configuration

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-external
spec:
  profile: Hardened
  replicas: 3

  tls:
    enabled: true
    mode: External

  # Configure external access (pick one).
  gateway:
    enabled: true
    hostname: "bao.example.com"
    gatewayRef:
      name: main-gateway
      namespace: gateway-system

  unseal:
    type: transit
    credentialsSecretRef:
      name: transit-token
    transit:
      address: "https://infra-bao.infra.svc:8200"
      mountPath: "transit"
      keyName: "openbao-unseal"
      tlsCACert: "/etc/bao/seal-creds/ca.crt"

  selfInit:
    enabled: true

  storage:
    size: 10Gi
```

!!! note
    When your externally-managed certificate does not include an IP SAN for `127.0.0.1`, the OpenBao Operator sets probe SNI automatically based on the configured Gateway/Ingress/Service hostname.