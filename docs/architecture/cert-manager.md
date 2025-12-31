# CertManager (TLS Lifecycle)

**Responsibility:** Bootstrap PKI and Rotate Certificates (or wait for external Secrets in External mode, or skip reconciliation in ACME mode).

The CertManager supports three modes controlled by `spec.tls.mode`:

## OperatorManaged Mode (Default)

When `spec.tls.mode` is `OperatorManaged` (or omitted), the operator generates and manages certificates:

**Logic Flow:**

1. **Reconcile:** Check if `Secret/<cluster>-tls-ca` exists.
2. **Bootstrap:** If missing, generate ECDSA P-256 Root CA. Store PEM in Secret.
3. **Issue:** Check if `Secret/<cluster>-tls-server` exists and is valid (for example, expiry > configured rotation window).
4. **Rotate:** If the server certificate is within the rotation window, regenerate it using the CA and update the Secret atomically.
5. **Trigger:** Compute a SHA256 hash of the active server certificate and annotate OpenBao pods with it (for example, `openbao.org/tls-cert-hash`) so the operator and in-pod components can track which certificate each pod is using.
6. **Hot Reload:** When the hash changes, the operator uses a `ReloadSignaler` to update the annotation on each ready OpenBao pod. A wrapper binary running as pid 1 in the OpenBao container watches the mounted TLS certificate file and sends `SIGHUP` to the OpenBao process when it detects changes, avoiding the need for `pods/exec` privileges in the operator and eliminating the need for `ShareProcessNamespace`.

## External Mode

When `spec.tls.mode` is `External`, the operator does not generate or rotate certificates:

**Logic Flow:**

1. **Wait:** Check if `Secret/<cluster>-tls-ca` exists. If missing, log "Waiting for external TLS CA Secret" and return (no error).
2. **Wait:** Check if `Secret/<cluster>-tls-server` exists. If missing, log "Waiting for external TLS server Secret" and return (no error).
3. **Monitor:** When both secrets exist, compute the SHA256 hash of the server certificate.
4. **Hot Reload:** Trigger hot-reload via `ReloadSignaler` when the certificate hash changes (enables seamless rotation by cert-manager or other external providers).
5. **No Rotation:** Do not check expiry or attempt to rotate certificates; assume the external provider handles this.

## ACME Mode

When `spec.tls.mode` is `ACME`, OpenBao manages certificates internally via its native ACME client:

**Logic Flow:**

1. **Skip Reconciliation:** The operator does not generate, rotate, or monitor certificates. OpenBao handles certificate lifecycle internally.
2. **Configuration:** The operator renders ACME parameters (`tls_acme_ca_directory`, `tls_acme_domains`, `tls_acme_email`) directly in the listener configuration.
3. **No Secrets:** No TLS Secrets are created or mounted. Certificates are stored in-memory (or cached per `tls_acme_cache_path`) by OpenBao.
4. **No Wrapper Needed:** No TLS reload wrapper is needed, and `ShareProcessNamespace` is disabled for better container isolation.
5. **Automatic Rotation:** OpenBao automatically handles certificate acquisition and rotation via the ACME protocol.

## Reconciliation Semantics

- **Idempotency:** Re-running reconciliation with the same Spec MUST lead to the same Secrets and annotations.
- **Backoff:** Transient errors (e.g., failed Secret update due to concurrent modification) are retried with exponential backoff.
- **Conditions:** Failures update a `TLSReady=False` condition with a clear reason and message.
- **External Mode:** The operator waits gracefully for external Secrets without erroring, allowing cert-manager or other tools time to provision certificates.

See also: [TLS Security](../security/tls.md)
