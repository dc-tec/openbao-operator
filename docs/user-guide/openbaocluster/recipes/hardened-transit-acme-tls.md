# Recipe: Hardened + Transit + ACME TLS

## Introduction

This recipe shows a production-style configuration using:
- `spec.profile: Hardened`
- Transit auto-unseal
- `tls.mode: ACME` (OpenBao native ACME client)

## Prerequisites

- A Transit-capable OpenBao instance reachable from the cluster (for unseal).
- An ACME CA (public or private) reachable from the cluster.
- NetworkPolicy egress/ingress rules allowing required connectivity.

## Configuration

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-acme
spec:
  profile: Hardened
  replicas: 3

  tls:
    enabled: true
    mode: ACME
    acme:
      directoryURL: "https://acme-v02.api.letsencrypt.org/directory"
      domains:
        - "bao.example.com"
      email: "admin@example.com"

  # For private ACME CAs: mount the directory TLS CA and the issuing PKI CA.
  # The PKI CA must be available as pki-ca.crt in the same volume directory.
  # configuration:
  #   acmeCARoot: "/etc/bao/seal-creds/ca.crt"

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

!!! tip
    For ACME + Gateway API, configure passthrough (`spec.gateway.tlsPassthrough: true`) and ensure the Gateway has a TLS listener in Passthrough mode.

!!! note
    For in-cluster private ACME CAs, prefer an internal `.svc` domain in `spec.tls.acme.domains` to avoid DNS surprises.