# 2. Security Profiles

[Back to User Guide index](README.md)

**IMPORTANT FOR PRODUCTION:** Always use the `Hardened` profile for production deployments. The `Development` profile stores root tokens in Kubernetes Secrets, which poses a significant security risk. See the [Hardened Profile](#22-hardened-profile) section for details.

The Operator supports two security profiles via `spec.profile`:

### 2.0 Optional AppArmor

AppArmor is **opt-in** because it is not available in all Kubernetes environments. When enabled, the operator sets `RuntimeDefault` AppArmor profiles on generated Pods/Jobs.

```yaml
spec:
  workloadHardening:
    appArmorEnabled: true
```

### 2.1 Development Profile (Default)

The `Development` profile allows relaxed security for development and testing:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: dev-cluster
spec:
  profile: Development  # Default, can be omitted
  version: "2.4.4"
  # ... other fields ...
```

**Characteristics:**
- Allows operator-managed TLS (`spec.tls.mode: OperatorManaged`)
- Allows static unseal (default)
- Allows self-init to be disabled (root token created)
- Sets `ConditionSecurityRisk=True` to indicate relaxed posture

**Use Case:** Development, testing, and non-production environments.

**WARNING:** The Development profile stores root tokens in Kubernetes Secrets, which poses a security risk. **DO NOT use the Development profile in production.** Always use the Hardened profile for production deployments.

### 2.2 Hardened Profile

**IMPORTANT: The Hardened profile is REQUIRED for production deployments.** The Development profile stores root tokens in Kubernetes Secrets, which poses a significant security risk in production environments. Always use the Hardened profile for production workloads.

The `Hardened` profile enforces strict security requirements for production:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
spec:
  profile: Hardened  # REQUIRED for production
  version: "2.4.4"
  tls:
    enabled: true
    mode: External  # Required for Hardened profile
  unseal:
    type: awskms  # Required: external KMS only
    options:
      region: us-east-1
      kms_key_id: alias/openbao-unseal
  selfInit:
    enabled: true  # Required for Hardened profile
    requests:
      # Your initialization requests
      - name: enable-audit
        operation: update
        path: sys/audit/file
        data:
          type: file
          options:
            file_path: /tmp/audit.log
```

**Requirements:**
- `spec.tls.mode` MUST be `External` (cert-manager or CSI managed) OR `ACME` (OpenBao native ACME client)
- `spec.unseal.type` MUST be external KMS (`awskms`, `gcpckms`, `azurekeyvault`, or `transit`)
- `spec.selfInit.enabled` MUST be `true` (no root token)
- `tls_skip_verify` in unseal options is rejected

**Automatic Features:**
- JWT authentication is automatically bootstrapped during self-init
- No manual JWT configuration required

**Security Benefits:**
- **No Root Tokens:** Root tokens are never created or stored, eliminating the risk of token compromise
- **External KMS:** Stronger root of trust than Kubernetes Secrets for unseal keys
- **External TLS:** Integrates with organizational PKI and certificate management
- **Automatic JWT Bootstrap:** Reduces configuration errors and ensures proper authentication setup

**Use Case:** **Production environments requiring maximum security. The Hardened profile MUST be used for all production deployments.**
