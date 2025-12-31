# Supply Chain Security

The Operator implements container image signature verification to protect against compromised registries, man-in-the-middle attacks, and TOCTOU (Time-of-Check to Time-of-Use) vulnerabilities.

## Image Verification

- **Verification Method:** Uses Cosign to verify container image signatures against a trusted public key.
- **Configuration:** Enabled via `spec.imageVerification.enabled` with a public key provided in `spec.imageVerification.publicKey`.
- **Timing:** Verification occurs before StatefulSet creation or updates, blocking deployment of unverified images when `failurePolicy` is `Block`.

## Rekor Transparency Log

By default, signatures are verified against the Rekor transparency log (`ignoreTlog: false`) to provide non-repudiation guarantees, following OpenBao's verification guidance.

- **Non-Repudiation:** Rekor verification makes it impossible to deny that a signature was created.
- **Disable Option:** Can be disabled via `spec.imageVerification.ignoreTlog: true` if needed.

## Digest Pinning (TOCTOU Mitigation)

The operator resolves image tags to immutable digests during verification and uses the verified digest in StatefulSets instead of the mutable tag.

- **Attack Prevention:** Prevents an attacker from updating a tag to point to a malicious image between verification and deployment.
- **Immutability:** Ensures the exact verified image is deployed.

## Private Registry Support

When `spec.imageVerification.imagePullSecrets` is provided, the operator uses these secrets to authenticate with private registries during verification.

- **Secret Types:** Secrets must be of type `kubernetes.io/dockerconfigjson` or `kubernetes.io/dockercfg`.

## Caching

Verification results are cached in-memory keyed by image digest (not tag) and public key to avoid redundant network calls while preventing cache issues when tags change.

## Failure Policies

| Policy | Behavior |
|--------|----------|
| `Block` (default) | Prevents StatefulSet updates and sets `ConditionDegraded=True` with `Reason=ImageVerificationFailed` |
| `Warn` | Logs an error and emits a Kubernetes Event but proceeds with deployment |

## Security Benefits

- **Supply Chain Protection:** Ensures that only cryptographically verified images are deployed.
- **Non-Repudiation:** Rekor verification provides proof of signature creation.
- **TOCTOU Prevention:** Digest pinning prevents tag-swapping attacks.
