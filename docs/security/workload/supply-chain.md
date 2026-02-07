# Supply Chain Security

!!! abstract "Immutable Assurance"
    The Operator implements container image signature verification to protect against compromised registries, man-in-the-middle attacks, and TOCTOU (Time-of-Check to Time-of-Use) vulnerabilities.

## Verification Flow

The Operator verifies images during reconciliation before it creates or updates operator-managed workloads
(StatefulSets and Jobs). Verification checks signatures against a trusted public key or keyless identity and
(optionally) the Rekor transparency log, then pins the image to an immutable digest.

```mermaid
flowchart LR
    Registry[(Container Registry)]
    Operator{{OpenBao Operator}}
    Rekor[Rekor Log]
    Cluster[Kubernetes Cluster]

    Registry --"1. Resolve Tag to Digest"--> Operator
    Operator --"2. Verify Signature"--> Operator
    Operator -.->|"3. Verify Rekor (Optional)"| Rekor

    Operator --"4. Reconcile Managed Workload with Verified Digest"--> Cluster
    
    classDef read fill:transparent,stroke:#60a5fa,stroke-width:2px,color:#fff;
    classDef write fill:transparent,stroke:#22c55e,stroke-width:2px,color:#fff;
    classDef process fill:transparent,stroke:#9333ea,stroke-width:2px,color:#fff;

    class Registry,Rekor read;
    class Operator process;
    class Cluster write;
```

## Configuration

=== ":material-signature-freehand: Image Verification"

    The Operator uses **[Cosign](https://github.com/sigstore/cosign){:target="_blank"}** to verify signatures.

    -   **Public Key:** You provide the public key; the Operator uses it to verify signatures found in the registry.
    -   **Private Registry:** Supports `imagePullSecrets` for authenticated verification.
    -   **Caching:** Results are cached in-memory by digest to prevent performance impact.

    !!! danger "Private Keys"
        The Operator **never** requires your private key. Signing happens in your CI/CD pipeline; the Operator only needs the *public* key for verification.

    ```yaml
    spec:
      imageVerification:
        enabled: true
        publicKey: |
          -----BEGIN PUBLIC KEY-----
          ...
          -----END PUBLIC KEY-----
    ```

=== ":material-pin: Digest Pinning"

    To prevent **TOCTOU** (Time-of-Check to Time-of-Use) attacks, the Operator mutates image tags to immutable digests.

    -   **Attack Vector:** An attacker pushes a malicious image to `1.2.3` *after* verification but *before* the Kubelet pulls it.
    -   **Mitigation:** The Operator resolves `openbao:1.2.3` to `openbao@sha256:abc...` during verification and forces the Pod to use the digest.

    !!! success "Immutability"
        This ensures that the *exact* bits that were verified are the ones that run in your cluster.

## Admission Guardrails (Optional, Defense-in-Depth)

Use a Kubernetes `ValidatingAdmissionPolicy` to enforce digest-only image references on operator-managed
resources. This catches bypass attempts where a mutable tag is submitted directly to the API.

By default (when `admissionPolicies.enabled=true`), Hardened managed workloads are marked for digest
enforcement and mutable tag references are denied for those workloads.

!!! note "Scope of Admission Policy"
    Admission policy is a guardrail, not a replacement for signature verification.
    Keep signature and Rekor verification in reconciliation, where the operator performs Cosign checks and
    pins verified digests.

## Rekor Transparency Log

By default, the Operator verifies signatures against the [Sigstore Rekor](https://docs.sigstore.dev/rekor/overview/){:target="_blank"} transparency log.

- **Non-Repudiation:** Ensures that the signature was actually created by the signer at a specific time.
- **Auditability:** Publicly meaningful event log of all signing activity.

!!! note "Air-Gapped Environments"
    In disconnected environments where reaching the public Rekor log is impossible, you can disable this check:
    ```yaml
    spec:
      imageVerification:
        ignoreTlog: true
    ```

## Failure Policies

| Policy | Behavior | Use Case |
| :--- | :--- | :--- |
| **Block** (Default) | **Prevents** the operator from reconciling managed workloads with unverified images. Sets `ConditionDegraded=True`. | Production environments requiring strict security. |
| **Warn** | Logs an error but **allows** reconciliation to continue using the original tag/reference. | Testing or during initial rollout of signing infrastructure. |

## Verified Workloads

Verification applies to all images managed by the Operator:

| Image | Config Field | Description |
| :--- | :--- | :--- |
| **OpenBao Server** | `spec.imageVerification` | The main OpenBao binary |
| **Init Container** | `spec.operatorImageVerification` | Helper for config rendering |
| **Backup/Restore Jobs** | `spec.operatorImageVerification` | Snapshot executors |
| **Upgrade Jobs** | `spec.operatorImageVerification` | Raft membership jobs |

## Separate Signers for OpenBao and Operator Images

The OpenBao main image (`openbao/openbao`) is signed by the **OpenBao project**, while helper images (init container, backup/restore executors, upgrade jobs) are signed by the **operator project**. Use `operatorImageVerification` to specify different signing credentials:

```yaml
spec:
  # Main OpenBao image (signed by openbao/openbao)
  image: "openbao/openbao:2.4.4"
  imageVerification:
    enabled: true
    issuer: "https://token.actions.githubusercontent.com"
    subject: "https://github.com/openbao/openbao/.github/workflows/release.yml@refs/tags/v2.4.4"
    failurePolicy: Block

  # Operator images (signed by dc-tec/openbao-operator)
  operatorImageVerification:
    enabled: true
    issuer: "https://token.actions.githubusercontent.com"
    subject: "https://github.com/dc-tec/openbao-operator/.github/workflows/release.yml@refs/tags/1.2.4"
    failurePolicy: Block

  initContainer:
    image: "ghcr.io/dc-tec/openbao-init:1.2.4"
  backup:
    image: "ghcr.io/dc-tec/openbao-backup:1.1.0"
```

!!! important "No Fallback Behavior"
    `operatorImageVerification` and `imageVerification` are completely independent configurations.
    If `operatorImageVerification` is not configured, helper images are **not verified** (even if `imageVerification` is set).
    This prevents confusing failures when the main image and helper images have different signers.

## Release Supply Chain

The operator project publishes release artifacts with a "build once, promote by digest" model:

- Images are built and tested under a `build-<sha>` tag.
- Stable/prerelease tags (for example `0.1.0`, `0.2.0-rc.1`) are promoted **by digest** (no rebuild).
- Images and charts are signed keylessly with Sigstore.
- SBOMs are generated and checksummed; the checksums file is signed as a blob.

=== ":material-check-decagram: Verify Operator Images"

    ```sh
    cosign verify \
      --certificate-identity-regexp "https://github.com/dc-tec/openbao-operator/.github/workflows/release.yml" \
      --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
      ghcr.io/dc-tec/openbao-operator:0.1.0
    ```

=== ":material-chart-bubble: Verify Helm Chart (OCI)"

    ```sh
    crane digest ghcr.io/dc-tec/charts/openbao-operator:0.1.0

    cosign verify \
      --certificate-identity-regexp "https://github.com/dc-tec/openbao-operator/.github/workflows/release.yml" \
      --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
      ghcr.io/dc-tec/charts/openbao-operator@sha256:...
    ```

=== ":material-file-lock: Verify Release Checksums"

    ```sh
    cosign verify-blob \
      --certificate checksums.txt.crt \
      --signature checksums.txt.sig \
      --certificate-identity-regexp "https://github.com/dc-tec/openbao-operator/.github/workflows/release.yml" \
      --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
      checksums.txt
    ```
