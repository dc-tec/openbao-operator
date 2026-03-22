# Security Considerations

Securing an OpenBao cluster involves careful management of initialization tokens, unseal keys, and container integrity. This guide outlines critical configurations for a production-hardened deployment.

## Root Token Management

During cluster initialization (bootstrap), OpenBao generates an initial **Root Token** with unlimited privileges. Handling this token securely is critical.

<Callout type="warning" title="Root Token Risk">

When self-init is disabled, the Operator stores the root token in a Kubernetes Secret named `<cluster>-root-token`.
This is convenient for development but risky for production.

</Callout>

### Recommended: Self-Initialization

For production environments, we strongly recommend enabling **Self-Initialization**.

- **How it works:** The Operator injects a one-time configuration to set up auth methods and policies immediately after initialization.
- **Benefit:** The root token is **automatically revoked** by OpenBao itself after setup is complete. It never persists in a Secret.

[Learn more about Self-Initialization](configuration/self-init.md)

---

## Auto-Unseal Configuration

OpenBao requires an "unseal key" to decrypt its master key on startup. You must choose a strategy for managing this key.

<Tabs groupId="cloud-kms-on-prem-hybrid-development-static">

<TabItem value="cloud-kms" label="Cloud KMS">

Offload key management to a trusted cloud provider. This is the **most secure** option for cloud deployments.

<Tabs groupId="aws-kms-gcp-cloud-kms-azure-key-vault">

<TabItem value="aws-kms" label="AWS KMS">

```yaml
spec:
  serviceAccount:
    annotations:
      eks.amazonaws.com/role-arn: "arn:aws:iam::123456789012:role/openbao-awskms"
  unseal:
    type: awskms
    awskms:
      kmsKeyID: "arn:aws:kms:us-east-1:123456789012:key/..."
      region: "us-east-1"
      # Optional: Use specific credentials if not using IRSA
      # accessKey: "..."
      # secretKey: "..."
```

</TabItem>

<TabItem value="gcp-cloud-kms" label="GCP Cloud KMS">

```yaml
spec:
  serviceAccount:
    annotations:
      iam.gke.io/gcp-service-account: "openbao@my-project.iam.gserviceaccount.com"
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

</TabItem>

<TabItem value="azure-key-vault" label="Azure Key Vault">

```yaml
spec:
  serviceAccount:
    annotations:
      azure.workload.identity/client-id: "87654321-4321-4321-4321-210987654321"
  podMetadata:
    labels:
      azure.workload.identity/use: "true"
  unseal:
    type: azurekeyvault
    azureKeyVault:
      vaultName: "my-vault"
      keyName: "openbao-key"
      # Optional: Specific tenant/client config
      # tenantID: "..."
      # clientID: "..."
```

</TabItem>

</Tabs>

<Callout type="note" title="Workload Identity Metadata">

For cloud-managed identity on the main OpenBao Pods, configure:

- `spec.serviceAccount.annotations` for ServiceAccount-bound identities such as EKS IRSA or GKE Workload Identity
- `spec.podMetadata.labels` when the platform also requires Pod labels, such as Azure Workload Identity

</Callout>

<Tabs groupId="oci-kms">

<TabItem value="oci-kms" label="OCI KMS">

```yaml
spec:
  unseal:
    type: ocikms
    ocikms:
      keyID: "ocid1.key.oc1..."
      cryptoEndpoint: "https://kms.<region>.oraclecloud.com"
      managementEndpoint: "https://kms.<region>.oraclecloud.com"
      # Default principal flow (for example instance principal):
      # authTypeAPIKey: false
      #
      # Or enable API-key mode and mount OCI SDK config via spec.unseal.credentialsSecretRef:
      # authTypeAPIKey: true
      #
      # The Secret must contain:
      # - config: OCI SDK config file with a [DEFAULT] profile
      # - the private key file referenced by key_file in that config
```

</TabItem>

</Tabs>

</TabItem>

<TabItem value="on-prem-hybrid" label="On-Prem / Hybrid">

Use existing hardware security modules or a central OpenBao/Vault cluster.

<Tabs groupId="transit-recommended-pkcs-11-hsm-kmip">

<TabItem value="transit-recommended" label="Transit (Recommended)">

Use another OpenBao cluster (the "provider") to unseal this cluster (the "dependent"). Ideally suited for multi-cluster management.

```yaml
spec:
  unseal:
    type: transit
    credentialsSecretRef:
      name: transit-unseal-creds
    transit:
      address: "https://central-openbao.example.com"
      keyName: "tenant-1-key"
      mountPath: "transit"
      # Optional: TLS verification
      # tlsSkipVerify: false
```

The referenced Secret should contain the transit token under key `token`. For custom CA or client mTLS, add the matching files to the same Secret and reference those file paths from the transit stanza.

</TabItem>

<TabItem value="pkcs-11-hsm" label="PKCS#11 (HSM)">

Connect to a physical Hardware Security Module (HSM).

```yaml
spec:
  unseal:
    type: pkcs11
    pkcs11:
      lib: "/usr/lib/libnotHSM.so" # Path to vendor library
      tokenLabel: "openbao-token"  # Use slot or tokenLabel
      pin: "1234"                  # User PIN
      keyLabel: "openbao-hsm-key"
      mechanism: "0x0009"          # Optional specific mechanism
      rsaOAEPHash: "sha256"        # Optional OAEP hash override
```

</TabItem>

<TabItem value="kmip" label="KMIP">

Connect to an enterprise Key Management Interoperability Protocol server.

```yaml
spec:
  unseal:
    type: kmip
    kmip:
      endpoint: "10.0.0.5:5696"
      kmsKeyID: "openbao-kmip-key"
      clientCert: "/etc/openbao/kmip/client.crt"
      clientKey: "/etc/openbao/kmip/client.key"
      caCert: "/etc/openbao/kmip/ca.pem"
      serverName: "kmip.internal.example"
```

</TabItem>

</Tabs>

</TabItem>

<TabItem value="development-static" label="Development (Static)">

Store the unseal key in a Kubernetes Secret.

<Callout type="danger" title="Production Risk">

This method stores the decryption key (`<cluster>-unseal-key`) alongside the encrypted data. If an attacker gains access to etcd or the namespace Secrets, they can decrypt the entire cluster.

**Requirements for safety:**
1. Enable **Etcd Encryption** in your Kubernetes cluster.
2. Strictly limit RBAC access to Secrets.

</Callout>

```yaml
spec:
  unseal:
    type: static  # Default
```

</TabItem>

</Tabs>

---

## Supply Chain Security

To protect against compromised container registries, the Operator includes native support for **Cosign** image verification.

<Callout type="success" title="Secure by Default">

The Operator verifies all images against the Rekor transparency log unless explicitly disabled.

</Callout>

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

For official release images, you can also use keyless verification without explicitly setting identity fields.
When `enabled: true` and `publicKey`/`issuer`/`subject`/`issuerRegExp`/`subjectRegExp` are omitted, the operator applies GitHub OIDC defaults
for known OpenBao/OpenBao-Operator release image repositories.

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

<Callout type="tip" title="OpenShift Users">

On OpenShift, the operator defaults to platform auto-detection and will omit pinned IDs in generated Pods/Jobs. If needed, force OpenShift mode via `OPERATOR_PLATFORM=openshift` (or `--platform=openshift`).

</Callout>

