# Workload Security

!!! abstract "Runtime Protection"
    Workload security focuses on the runtime aspect of the OpenBao deployment: securing the Pods, Containers, and Images that make up the service.

## Overview

The Operator enforces a **Secure-by-Default** runtime environment:

1. **Pod Security:** Strict non-root, read-only filesystem containers compliant with `Restricted` PSS.
2. **TLS:** Automated certificate management for end-to-end encryption.
3. **Supply Chain:** Cryptographic verification of container images to prevent tampering.

## Topics

<div class="grid cards" markdown>

- :material-docker: **Pod Security**

    ---

    Deep dive into SecurityContexts, ServiceAccount tokens, and Resource Limits.

    [:material-arrow-right: Container Hardening](workload-security.md)

- :material-certificate: **TLS Management**

    ---

    Managing server TLS, mutual TLS (mTLS) for peers, and integration with Cert-Manager.

    [:material-arrow-right: TLS Configuration](tls.md)

- :material-signature: **Supply Chain**

    ---

    Verifying image signatures with Cosign and enforcing digest pinning.

    [:material-arrow-right: Image Verification](supply-chain.md)

</div>

## Default Security Posture

All OpenBao Pods are deployed with the following non-negotiable settings:

```yaml
securityContext:
  runAsUser: 1000
  runAsGroup: 1000
  runAsNonRoot: true
  readOnlyRootFilesystem: true
  allowPrivilegeEscalation: false
  capabilities:
    drop: ["ALL"]
  seccompProfile:
    type: RuntimeDefault
```

## See Also

- [:material-shield-check: Security Favorites](../fundamentals/index.md)
- [:material-server-network: Infrastructure Security](../infrastructure/index.md)
