# Production Checklist

Before deploying OpenBao Operator in production, complete this checklist to ensure a secure, reliable, and compliant deployment.

## Critical Security

!!! danger "Security Hardening (Required)"
    Failure to configure these settings puts your cluster at significant risk.

    - [ ] **Hardened Profile**: Set `spec.profile: Hardened` to enforce secure defaults.
        - [Learn more](../configuration/security-profiles.md)
    - [ ] **External Root of Trust**: Use a non-static external seal such as `transit`, cloud KMS, `ocikms`, `kmip`, or `pkcs11`. Do **NOT** use static unseal keys in Kubernetes Secrets for production.
        - [Learn more](../configuration/self-init.md)
    - [ ] **Etcd Encryption**: Ensure your Kubernetes cluster enables encryption at rest for Secrets (where unseal keys might be stored).
    - [ ] **TLS Mode**: Use `ACME` (Let's Encrypt) or `External` (Custom CA). Avoid `OperatorManaged` for public-facing endpoints.
        - [Learn more](../../../security/workload/tls.md)
    - [ ] **Self-Initialization**: Enable `spec.selfInit` to prevent the initial root token from ever being surfaced to the operator or logs.
        - **CRITICAL**: Configure `spec.selfInit.requests` with user authentication methods (e.g., userpass, JWT, Kubernetes auth) to prevent lockout.
        - Enable `spec.selfInit.oidc.enabled: true` for Operator lifecycle authentication (backups, upgrades) - this does NOT provide user authentication.
        - [Learn more](../configuration/self-init.md)

!!! warning "Admission Control"
    Without these policies, tenant isolation cannot be guaranteed.

    - [ ] **ValidatingAdmissionPolicies**: Verify that the full required dependency set is installed and enforced, including:
        - `openbao-validate-openbaocluster`
        - `openbao-validate-openbao-tenant`
        - `openbao-validate-openbaorestore`
        - `openbao-lock-controller-statefulset-mutations`
        - `openbao-lock-managed-resource-mutations`
        - `openbao-enforce-managed-image-digests`
        - `openbao-restrict-provisioner-rbac`
        - `openbao-restrict-provisioner-namespace-mutations`
        - `openbao-restrict-provisioner-tenant-governance`
        - `openbao-restrict-controller-rbac`
        - `openbao-restrict-controller-secret-writes`
        - [Learn more](../../../security/infrastructure/admission-policies.md)

## Reliability & Scale

!!! tip "Resource Planning"
    - [ ] **Resources**: Set explicit `requests` and `limits`. Minimum **256Mi** memory for small clusters; scale CPU based on expected request rate.
        - [Learn more](../configuration/resources-storage.md)
    - [ ] **Storage Class**: Use a high-performance (SSD), low-latency StorageClass and set `spec.storage.storageClassName` explicitly on new clusters. Raft requires low fsync latency, and the effective storage class is immutable after PVC creation.
    - [ ] **Volume Size**: Plan for growth. Raft snapshots can consume significant space.

!!! tip "Availability"
    - [ ] **Topology Spread**: Ensure your `Kubernetes` cluster has nodes in multiple zones. The Operator adds soft node and zone spread constraints plus preferred anti-affinity.
    - [ ] **Replica Count**: Use at least **3 replicas** for high availability.

## Day 2 Operations

!!! note "Operational Readiness"
    - [ ] **Backups**: Configure scheduled backups to S3/GCS. **Test a restore** before going live.
        - [Learn more](backups.md)
    - [ ] **Network Policy**: Verify `egressRules` allow access to necessary external services (Cloud KMS, S3, OIDC providers).
        - [Learn more](../../../security/infrastructure/network-security.md)
    - [ ] **Monitoring**: Ensure metrics scraping is configured and secured.
        - [ ] **Scraping**: Use Prometheus Operator (`ServiceMonitor`) or VictoriaMetrics Operator (`VMServiceScrape`) for operator metrics.
        - [ ] **RBAC**: Grant your scraper ServiceAccount permission to GET `/metrics` (Helm: `metrics.rbac.enabled: true` with `metrics.rbac.subjects`).
        - [ ] **TLS Verification**: Prefer strict TLS verification in production.
            - Set `metrics.serviceMonitor.tlsConfig.insecureSkipVerify: false` (and configure CA/certs), or
            - Set `metrics.victoriaMetrics.tlsConfig.insecureSkipVerify: false` (and configure trusted CA in your scraping stack).
        - [Learn more](../configuration/observability.md)
    - [ ] **Logs**: Verify structured logs (`cluster_name`, `cluster_namespace`) are reaching your log aggregator.
        - [Learn more](../configuration/observability.md#logging)
    - [ ] **Alerts**: Configure alerts for backup staleness, cluster degradation, and reconciliation errors.
        - [Learn more](../configuration/observability.md#recommended-alerts)

## Final Verification

Check the cluster status one last time before routing traffic:

```sh
kubectl describe openbaocluster <name> -n <namespace>
```

**Success Criteria:**

- [ ] Condition `ProductionReady` is **True**.
- [ ] Condition `Available` is **True**.
- [ ] Relevant integration conditions are healthy for your topology:
    - `CloudUnsealIdentityReady`
    - `GatewayIntegrationReady`
    - `APIServerNetworkReady`
    - `BackupConfigurationReady`
- [ ] `Status.Phase` is **Running**.
