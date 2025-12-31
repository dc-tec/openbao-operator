# Production Checklist

Before deploying OpenBao Operator in production, complete this checklist to ensure a secure and reliable deployment.

## Security Profile

- [ ] **Set `spec.profile: Hardened`** in your `OpenBaoCluster` manifest
  - Development profile is insecure and emits warnings
  - See [Security Profiles](security-profiles.md) for details

## Unseal Configuration

- [ ] **Configure external unseal** (do not use static auto-unseal in production)
  - Recommended: Transit unseal with external OpenBao/Vault
  - Alternative: Cloud KMS (AWS KMS, GCP KMS, Azure Key Vault)
  - See [Self-Initialization](self-init.md) for setup

- [ ] **Verify etcd encryption at rest** is enabled on your cluster
  - Static unseal keys are stored in Kubernetes Secrets
  - Without etcd encryption, secrets are stored in plaintext
  - Consult your Kubernetes distribution's documentation

## TLS Configuration

- [ ] **Choose appropriate TLS mode**:
  - `OperatorManaged`: Operator generates and rotates internal CA (simplest)
  - `External`: Bring your own certificates (for compliance requirements)
  - `ACME`: Automated certificates via cert-manager (for public endpoints)
  - See [TLS Configuration](../security/workload/tls.md) for details

## Self-Initialization

- [ ] **Enable self-init** to avoid root token Secrets
  - Configure: `spec.selfInit.enabled: true`
  - Prevents root token from being stored in Kubernetes
  - See [Self-Initialization](self-init.md) for complete setup

## Admission Policies

- [ ] **Verify ValidatingAdmissionPolicies are installed and enforced**
  - Required for Sentinel security
  - Policies: `openbao-restrict-sentinel-mutations`, `openbao-restrict-provisioner-delegate`
  - See [Admission Policies](../security/infrastructure/admission-policies.md)

## Network Security

- [ ] **Review NetworkPolicy configuration**
  - Default policies enforce deny-all ingress
  - Configure egress rules for backup targets and external services
  - See [Network Security](../security/infrastructure/network-security.md)

## Backup Configuration

- [ ] **Configure scheduled backups** to object storage
  - S3, GCS, or Azure Blob Storage
  - Test restore procedures before production
  - See [Backups](backups.md) and [Restore](restore.md)

- [ ] **Verify backup credentials** are properly secured
  - Use cloud provider workload identity where possible
  - Rotate credentials regularly

## Resource Planning

- [ ] **Set appropriate resource requests and limits**
  - Memory: Minimum 256Mi for small deployments
  - CPU: Scale based on request volume
  - See [Resources & Storage](resources.md)

- [ ] **Size storage appropriately**
  - Raft storage grows with secret count
  - Plan for headroom and backup retention

## Observability

- [ ] **Verify metrics are being collected**
  - Metrics use `openbao_` prefix
  - Configure alerting for key conditions

- [ ] **Review operator logs** are being collected
  - Structured logging with `cluster_namespace`, `cluster_name` fields

## Recovery Preparation

- [ ] **Familiarize with recovery runbooks**:
  - [Break Glass / Safe Mode](recovery-safe-mode.md)
  - [No Leader / No Quorum](recovery-no-leader.md)
  - [Sealed Cluster](recovery-sealed-cluster.md)
  - [Failed Rollback](recovery-failed-rollback.md)

## Final Verification

- [ ] **Check `ProductionReady` condition** on your cluster:

  ```sh
  kubectl describe openbaocluster <name> -n <namespace>
  ```

  - Condition should be `True`
  - If `False`, review the reason and message for remediation steps
