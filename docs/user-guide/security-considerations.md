# Security Considerations

## Root Token Secret

During cluster initialization (when **not** using self-initialization), the Operator stores the initial root token in a Secret named `<cluster>-root-token`. This token grants **full administrative access** to the OpenBao cluster.

**Note:** When `spec.selfInit.enabled = true`, no root token Secret is created. The root token is automatically revoked by OpenBao after self-initialization completes. See section 5 for details.

Retrieve the root token (for initial setup only, standard init only):

```sh
kubectl -n security get secret dev-cluster-root-token -o jsonpath='{.data.token}' | base64 -d
```

**Security Best Practices:**

- **Limit Access:** Configure RBAC to restrict access to the root token Secret. Only cluster administrators should have read access.
- **Rotate or Revoke:** After initial cluster setup, consider revoking the root token and creating more granular policies for ongoing administration.
- **Audit Access:** Enable Kubernetes audit logging to monitor access to this Secret.
- **Consider Self-Init:** For production workloads with GitOps workflows, consider using self-initialization to avoid having a root token Secret at all.
- **Not Auto-Deleted:** The root token Secret is NOT automatically deleted when the cluster is deleted (to support recovery scenarios). Clean it up manually if needed:

  ```sh
  kubectl -n security delete secret dev-cluster-root-token
  ```

## Auto-Unseal Configuration

### Static Auto-Unseal (Default)

The Operator manages a static auto-unseal key stored in `<cluster>-unseal-key`. This key is used by OpenBao to automatically unseal on startup.

**Important:** If this Secret is compromised, an attacker with access to the Raft data can decrypt it. Ensure:

- etcd encryption at rest is enabled in your Kubernetes cluster.
- RBAC strictly limits access to Secrets in the OpenBao namespace.

**OpenBao Version Requirement:** The static auto-unseal feature requires **OpenBao v2.4.0 or later**. Earlier versions do not support the `seal "static"` configuration and will fail to start.

### External KMS Auto-Unseal

For enhanced security, you can configure external KMS providers (AWS KMS, GCP Cloud KMS, Azure Key Vault, or OpenBao with the Transit Secret Engine) to manage the unseal key instead of storing it in Kubernetes Secrets.

#### AWS KMS

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
  namespace: security
spec:
  version: "2.1.0"
  image: "openbao/openbao:2.1.0"
  replicas: 3
  profile: Development
  unseal:
    type: "awskms"
    awskms:
      kmsKeyID: "arn:aws:kms:us-east-1:123456789012:key/abcd1234-5678-90ab-cdef-1234567890ab"
      region: "us-east-1"
    # Optional: If not using IRSA (IAM Roles for Service Accounts)
    credentialsSecretRef:
      name: aws-kms-credentials
      namespace: security
  tls:
    enabled: true
    rotationPeriod: "720h"
  storage:
    size: "10Gi"
```

#### GCP Cloud KMS

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
  namespace: security
spec:
  version: "2.1.0"
  image: "openbao/openbao:2.1.0"
  replicas: 3
  profile: Development
  unseal:
    type: "gcpckms"
    gcpCloudKMS:
      project: "my-gcp-project"
      region: "us-central1"
      keyRing: "openbao-keyring"
      crypto_key: "openbao-unseal-key"
    # Optional: If not using GKE Workload Identity
    credentialsSecretRef:
      name: gcp-kms-credentials
      namespace: security
  tls:
    enabled: true
    rotationPeriod: "720h"
  storage:
    size: "10Gi"
```

**Note:** When using external KMS, the operator does NOT create the `<cluster>-unseal-key` Secret. The Root of Trust shifts from Kubernetes Secrets to the cloud provider's KMS service. For GCP, the credentials Secret must contain a key named `credentials.json` with the GCP service account JSON credentials.

## Image Verification (Supply Chain Security)

To protect against compromised registries and supply chain attacks, you can enable container image signature verification using Cosign. The operator verifies image signatures against the Rekor transparency log by default, providing strong non-repudiation guarantees.

### Enable Image Verification

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
  namespace: security
spec:
  version: "2.1.0"
  image: "openbao/openbao:2.1.0"
  imageVerification:
    enabled: true
    publicKey: |
      -----BEGIN PUBLIC KEY-----
      MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE...
      -----END PUBLIC KEY-----
    failurePolicy: Block  # or "Warn" to log but proceed
    # ignoreTlog: false  # Set to true to disable Rekor transparency log verification
  replicas: 3
  profile: Development
  tls:
    enabled: true
    rotationPeriod: "720h"
  storage:
    size: "10Gi"
```

### Image Verification with Private Registry

When using images from private registries, provide ImagePullSecrets for authentication:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
  namespace: security
spec:
  version: "2.1.0"
  image: "private-registry.example.com/openbao/openbao:2.1.0"
  imageVerification:
    enabled: true
    publicKey: |
      -----BEGIN PUBLIC KEY-----
      MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE...
      -----END PUBLIC KEY-----
    failurePolicy: Block
    imagePullSecrets:
      - name: registry-credentials  # Secret of type kubernetes.io/dockerconfigjson
  replicas: 3
  profile: Development
  tls:
    enabled: true
    rotationPeriod: "720h"
  storage:
    size: "10Gi"
```

**Configuration Options:**

- `enabled`: Enable or disable image verification (required).
- `publicKey`: PEM-encoded Cosign public key used to verify signatures (required when enabled).
- `failurePolicy`: Behavior on verification failure:
  - `Block` (default): Blocks reconciliation of the affected workload and records an image verification failure condition.
  - `Warn`: Logs an error and emits a Kubernetes Event but proceeds with deployment.
- `ignoreTlog`: Set to `true` to disable Rekor transparency log verification (default: `false`). When `false`, signatures are verified against Rekor for non-repudiation, following OpenBao's verification guidance.
- `imagePullSecrets`: List of ImagePullSecrets (type `kubernetes.io/dockerconfigjson` or `kubernetes.io/dockercfg`) for private registry authentication during verification.

**Security Features:**

- **TOCTOU Mitigation**: The operator resolves image tags to digests during verification and uses the verified digest in operator-managed workloads (StatefulSets/Deployments/Jobs), preventing Time-of-Check to Time-of-Use attacks where a tag could be updated between verification and deployment.
- **Rekor Verification**: By default, signatures are verified against the Rekor transparency log, providing strong non-repudiation guarantees as recommended by OpenBao.
- **Private Registry Support**: ImagePullSecrets can be provided for authentication when verifying images from private registries.

**Note:** Image verification results are cached in-memory keyed by image digest (not tag) and public key to avoid redundant network calls while preventing cache issues when tags change.
