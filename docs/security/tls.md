# TLS & Identity

The Operator supports three modes for TLS certificate management.

## Operator-Managed TLS (Default)

When `spec.tls.mode` is `OperatorManaged` (or omitted), the Operator acts as an internal Certificate Authority (CA) to enforce mTLS.

- **Automated PKI:** The operator generates a self-signed Root CA and issues ephemeral leaf certificates for every cluster.
- **Strict SANs:** Certificates include strict Subject Alternative Names (SANs) for the Service and Pod DNS names. Pod IPs are explicitly *excluded* to avoid identity fragility during pod churn.
- **Rotation:** Server certificates are automatically rotated before expiry (configurable via `spec.tls.rotationPeriod`).
- **Gateway Integration:** When Gateway API is used, the operator manages a CA ConfigMap to allow the Gateway (e.g., Traefik) to validate the backend OpenBao pods, ensuring full end-to-end TLS.

## External TLS Provider

When `spec.tls.mode` is `External`, the operator does not generate or rotate certificates. Instead, it expects Secrets to be managed by an external entity (e.g., cert-manager, corporate PKI, or CSI drivers).

- **BYO-PKI:** Users can integrate with their organization's PKI infrastructure or use cert-manager for certificate lifecycle management.
- **Secret Names:** The operator expects Secrets named `<cluster-name>-tls-ca` and `<cluster-name>-tls-server` to exist in the cluster namespace.
- **Hot Reload:** The operator still monitors certificate changes and triggers hot-reloads when external providers rotate certificates, ensuring seamless certificate updates without service interruption.
- **No Rotation:** The operator does not check expiry or attempt to rotate certificates in External mode; this is the responsibility of the external provider.

## ACME TLS (Native OpenBao ACME Client)

When `spec.tls.mode` is `ACME`, OpenBao uses its native ACME client to automatically obtain and manage TLS certificates.

- **Native ACME:** OpenBao fetches certificates over the network using the ACME protocol (e.g., Let's Encrypt) and stores them in-memory (or cached per `tls_acme_cache_path`).
- **No Secrets:** No Kubernetes Secrets are created or mounted for server certificates. Certificates are managed entirely by OpenBao.
- **Automatic Rotation:** OpenBao handles certificate acquisition and rotation automatically via the ACME protocol, eliminating the need for external certificate management tools.
- **No Wrapper Needed:** No TLS reload wrapper is needed, and `ShareProcessNamespace` is disabled, providing better container isolation and security.
- **Zero Trust:** The operator never possesses private keys, making this mode ideal for zero-trust architectures.
- **Configuration:** ACME parameters (`directoryURL`, `domain`, `email`) are configured via `spec.tls.acme` and rendered directly in the OpenBao listener configuration.

## Comparison

| Feature | Operator-Managed | External | ACME |
| ------- | ---------------- | -------- | ---- |
| Certificate Generation | Operator | External provider | OpenBao |
| Rotation | Automatic | External provider | Automatic (ACME) |
| Private Key Location | Kubernetes Secret | External Secret | In-memory (OpenBao) |
| PKI Integration | Self-signed | Corporate PKI | Public CA (e.g., Let's Encrypt) |
| Best For | Development/Testing | Production with existing PKI | Zero-trust architectures |
