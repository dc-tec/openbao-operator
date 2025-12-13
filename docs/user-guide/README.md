# OpenBao Supervisor Operator – User Guide

This guide covers deploying the operator, provisioning tenant namespaces, and operating `OpenBaoCluster` resources.

If you are using the Provisioner, you must create an `OpenBaoTenant` in the operator namespace (typically `openbao-operator-system`) for each target namespace. See [3. Provision Tenant Namespaces](tenant-namespaces.md).

See also:
- [Architecture](../architecture.md)
- [Security](../security.md)
- [Contributing](../contributing.md)

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
- [10. Backup Configuration](backups.md)
- [11. Network Configuration](network.md)
- [12. Upgrades](upgrades.md)
- [13. Pausing Reconciliation](pausing.md)
- [14. Deletion and DeletionPolicy](deletion.md)
- [15. Security Considerations](security-considerations.md)
- [16. Multi-Tenancy Security](multi-tenancy.md)
- [17. Next Steps](next-steps.md)
