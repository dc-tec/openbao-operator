# OpenBao Supervisor Operator – User Guide

This guide covers deploying the operator, provisioning tenant namespaces, and operating `OpenBaoCluster` resources.

If you are using the Provisioner, you must create an `OpenBaoTenant` in the operator namespace (typically `openbao-operator-system`) for each target namespace. See [3. Provision Tenant Namespaces](tenant-namespaces.md).

See also:

- [Architecture](../architecture/index.md)
- [Security](../security/index.md)
- [Contributing](../contributing/index.md)

## Topics

- [1. Deploy the Operator](installation.md)
- [2. Security Profiles](security-profiles.md)
- [3. Provision Tenant Namespaces](tenant-namespaces.md)
- [4. Create a Basic OpenBaoCluster](cluster-basic.md)
- [5. What the Operator Creates](resources.md)
- [6. External Access (Service and Ingress)](external-access.md)
- [7. Self-Initialization](self-init.md)
- [8. Gateway API Support](gateway-api.md)
- [9. Advanced Configuration](advanced-configuration.md)
- [10. Sentinel Drift Detection](sentinel.md)
- [11. Backup Configuration](backups.md)
- [12. Network Configuration](network.md)
- [13. Upgrades](upgrades.md)
- [14. Pausing Reconciliation](pausing.md)
- [15. Deletion and DeletionPolicy](deletion.md)
- [16. Security Considerations](security-considerations.md)
- [17. Multi-Tenancy Security](multi-tenancy.md)
- [18. Next Steps](next-steps.md)
