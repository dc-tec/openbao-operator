# Security Considerations

Securing an OpenBao cluster involves careful management of initialization tokens, unseal keys, and container integrity. This guide outlines critical configurations for a production-hardened deployment.

## Root Token Management

During cluster initialization (bootstrap), OpenBao generates an initial **Root Token** with unlimited privileges. Handling this token securely is critical.

!!! warning "Root Token Risk"
    By default, the Operator stores the root token in a Kubernetes Secret named `<cluster>-root-token`. This is convenient for development but risky for production.

### Recommended: Self-Initialization

For production environments, we strongly recommend enabling **Self-Initialization**.

- **How it works:** The Operator injects a one-time configuration to set up auth methods and policies immediately after initialization.
- **Benefit:** The root token is **automatically revoked** by OpenBao itself after setup is complete. It never persists in a Secret.

[Learn more about Self-Initialization](configuration/self-init.md)

---

## Auto-Unseal Configuration

OpenBao requires an "unseal key" to decrypt its master key on startup. You must choose a strategy for managing this key.

=== "Cloud KMS"
    Offload key management to a trusted cloud provider. This is the **most secure** option for cloud deployments.

    === "AWS KMS"
        ```yaml
        spec:
          unseal:
            type: awskms
            awskms:
              kmsKeyID: "arn:aws:kms:us-east-1:123456789012:key/..."
              region: "us-east-1"
              # Optional: Use specific credentials if not using IRSA
              # accessKey: "..."
              # secretKey: "..."
        ```

    === "GCP Cloud KMS"
        ```yaml
        spec:
          unseal:
            type: gcpckms
            gcpCloudKMS:
              project: "my-project"
              region: "us-central1"
              keyRing: "openbao-ring"
              cryptoKey: "openbao-key"
              # Optional: Use specific credentials file if not using Workload Identity
              # credentials: "JSON_STRING_OR_PATH"
        ```

    === "Azure Key Vault"
        ```yaml
        spec:
          unseal:
            type: azurekeyvault
            azureKeyVault:
              vaultName: "my-vault"
              keyName: "openbao-key"
              # Optional: Specific tenant/client config
              # tenantID: "..."
              # clientID: "..."
        ```
    
    === "OCI KMS"
        ```yaml
        spec:
          unseal:
            type: ocikms
            ocikms:
              keyID: "ocid1.key.oc1..."
              cryptoEndpoint: "https://<unique>.crypto.objectstorage.<region>.oci.customer-oci.com"
              managementEndpoint: "https://<unique>.management.objectstorage.<region>.oci.customer-oci.com"
              authType: "instance_principal" # or "user_principal"
        ```

=== "On-Prem / Hybrid"
    Use existing hardware security modules or a central OpenBao/Vault cluster.

    === "Transit (Recommended)"
        Use another OpenBao cluster (the "provider") to unseal this cluster (the "dependent"). Ideally suited for multi-cluster management.

        ```yaml
        spec:
          unseal:
            type: transit
            transit:
              address: "https://central-openbao.example.com"
              token: "hvs.CAES..."  # Token with 'update' on transit/encrypt/key
              keyName: "tenant-1-key"
              mountPath: "transit"
              # Optional: TLS verification
              # tlsSkipVerify: false
        ```

    === "PKCS#11 (HSM)"
        Connect to a physical Hardware Security Module (HSM).

        ```yaml
        spec:
          unseal:
            type: pkcs11
            pkcs11:
              lib: "/usr/lib/libnotHSM.so" # Path to vendor library
              slot: "0"
              pin: "1234"                  # User PIN
              keyLabel: "openbao-hsm-key"
              hmacKeyLabel: "openbao-hsm-hmac-key"
              generateKey: true            # Generate if missing
              mechanism: "0x0000"          # Optional specific mechanism
        ```

    === "KMIP"
        Connect to an enterprise Key Management Interoperability Protocol server.

        ```yaml
        spec:
          unseal:
            type: kmip
            kmip:
              address: "10.0.0.5:5696"
              certificate: "/etc/openbao/kmip/cert.pem"
              key: "/etc/openbao/kmip/key.pem"
              caCert: "/etc/openbao/kmip/ca.pem"
        ```

=== "Development (Static)"
    Store the unseal key in a Kubernetes Secret.

    !!! danger "Production Risk"
        This method stores the decryption key (`<cluster>-unseal-key`) alongside the encrypted data. If an attacker gains access to etcd or the namespace Secrets, they can decrypt the entire cluster.

        **Requirements for safety:**
        1. Enable **Etcd Encryption** in your Kubernetes cluster.
        2. Strictly limit RBAC access to Secrets.

    ```yaml
    spec:
      unseal:
        type: static  # Default
    ```

---

## Supply Chain Security

To protect against compromised container registries, the Operator includes native support for **Cosign** image verification.

!!! success "Secure by Default"
    The Operator verifies all images against the Rekor transparency log unless explicitly disabled.

### Enabling Verification

Add the `imageVerification` block to your `OpenBaoCluster`. The Operator will block the startup of any Pods if the image signature cannot be verified against the public key.

```yaml
spec:
  imageVerification:
    enabled: true
    failurePolicy: Block  # "Block" or "Warn"
    publicKey: |
      -----BEGIN PUBLIC KEY-----
      MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE...
      ...
      -----END PUBLIC KEY-----
```

### Private Registries

If your images are in a private registry, provide the necessary pull secrets:

```yaml
spec:
  imageVerification:
    enabled: true
    publicKey: | ... |
    imagePullSecrets:
      - name: my-registry-creds
```

---

## Workload Isolation

By default, the operator configures Pods with a strict security context:
- **UID/GID:** `100:1000` (pinned to the `bao` user in official images)
- **Privileges:** `runAsNonRoot: true`, `allowPrivilegeEscalation: false`
- **Capabilities:** Drop `ALL`

### Platform Compatibility & Overrides

For platforms with strict admission controllers (e.g., OpenShift SCC) or custom requirements, you can override the Pod Security Context.

```yaml
spec:
  securityContext:
    # Example: Run as a specific UID provided by your security team
    runAsUser: 1001
    runAsGroup: 1001
    fsGroup: 1001
    
    # Example: Explicitly unset IDs to let the platform assign them (OpenShift behavior)
    # Note: On OpenShift, prefer leaving these unset and rely on the operator's platform auto-detection (default).
    # To force OpenShift mode, set OPERATOR_PLATFORM=openshift (or --platform=openshift).
    # runAsUser: null 
```

!!! tip "OpenShift Users"
    On OpenShift, the operator defaults to platform auto-detection and will omit pinned IDs in generated Pods/Jobs. If needed, force OpenShift mode via `OPERATOR_PLATFORM=openshift` (or `--platform=openshift`).
