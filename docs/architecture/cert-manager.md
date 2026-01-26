# Cert Manager (TLS Lifecycle)

**Responsibility:** Bootstrap PKI, Rotate Certificates, and signal Hot Reloads.

The CertManager ensures that OpenBao always has valid TLS certificates without requiring manual intervention or pod restarts.

## 1. Operating Modes

Controlled by `spec.tls.mode`.

| Feature | OperatorManaged (Default) | External | ACME |
| :--- | :--- | :--- | :--- |
| **CA Source** | Generated Self-Signed Root CA | User provided Secret | Let's Encrypt / ACME |
| **Server Cert** | Operator Generated (ECDSA P-256) | User provided Secret | OpenBao Internal |
| **Rotation** | **Automatic** (Operator) | External (cert-manager) | **Automatic** (OpenBao) |
| **Reload** | **Automatic** (Signal) | **Automatic** (Signal) | Internal |

---

## 2. Architecture

### TLS Rotation Loop (`OperatorManaged`)

allows the operator to rotate certificates **atomically** without downtime.

```mermaid
graph TD
    Check{Check Expiry} -->|Expired?| Generate[Generate New Cert]
    Generate -->|Update| Secret[("fa:fa-key TLS Secret")]
    
    Secret -.->|Watch| Operator
    Operator -->|Compute Hash| Annotation["Pod Annotation\n(openbao.org/tls-cert-hash)"]
    Annotation -->|Update| Pods[OpenBao Pods]
    
    classDef process fill:transparent,stroke:#9333ea,stroke-width:2px,color:#fff;
    classDef write fill:transparent,stroke:#22c55e,stroke-width:2px,color:#fff;
    classDef read fill:transparent,stroke:#60a5fa,stroke-width:2px,color:#fff;
    
    class Check,Generate process;
    class Secret,Annotation write;
    class Operator,Pods read;
```

### Hot Reload Mechanism

We avoid `ShareProcessNamespace` or `exec` privileges by using a lightweight wrapper process (PID 1).

```mermaid
sequenceDiagram
    participant Op as Operator
    participant K8s as Kubernetes
    participant Pod as Pod (PID 1 Wrapper)
    participant Bao as OpenBao (PID 2)

    Op->>K8s: Update Secret (New Cert)
    K8s->>Pod: Update Volume Mount (/etc/bao/tls/tls.crt)
    Pod->>Pod: Detect File Change (fsnotify)
    Pod->>Bao: Send SIGHUP
    Bao->>Bao: Reload Certificate
    
    note right of Op: No Pod Restart Required
```

---

## 3. Implementation Details

=== "OperatorManaged"

    **Best for:** Most users, development, and self-contained environments.

    1.  **Bootstrap:** Checks for `Secret/<cluster>-tls-ca`. If missing, generates a 10-year ECDSA P-256 Root CA.
    2.  **Issuance:** Checks `Secret/<cluster>-tls-server`. If missing or near expiry (default < 24h), signs a new certificate using the CA.
    3.  **Trust:** The CA is automatically mounted to all OpenBao pods and exported to a `ConfigMap` for clients.

=== "External"

    **Best for:** Production environments using `cert-manager` or external PKI.

    1.  **Passive:** The operator stops generating secrets.
    2.  **Wait:** It waits for `Secret/<cluster>-tls-ca` and `Secret/<cluster>-tls-server` to be created by an external tool.
    3.  **Signaling:** It continues to monitor the *content* of these secrets. When your external tool rotates the cert, the operator detects the hash change and triggers the hot-reload signal.

=== "ACME"

    **Best for:** Public-facing clusters needing trusted browser certificates.

    1.  **Passthrough:** The operator renders ACME configuration (`tls_acme_...`) directly into `config.hcl`.
    2.  **No Secrets:** Kubernetes Secrets are NOT used. OpenBao stores certs internally (or in a persistent cache).
    3.  **No Reload:** The Operator is effectively bypassed for TLS logic. OpenBao handles its own rotation loop.
