# OpenBao Operator – User Guide

This guide covers deploying the operator, provisioning tenant namespaces, and operating `OpenBaoCluster` resources.

## Getting Started

New to the OpenBao Operator? Start here:

1. [Installation](installation.md) - Deploy the operator to your cluster
2. [Basic Cluster](cluster-basic.md) - Create your first OpenBaoCluster
3. [Next Steps](next-steps.md) - Where to go from here

## Configuration

Customize your OpenBao deployment:

- [External Access](external-access.md) - Service and Ingress configuration
- [Network Configuration](network.md) - NetworkPolicies and custom rules
- [Resources & Storage](resources.md) - CPU, memory, and PVC settings
- [Advanced Configuration](advanced-configuration.md) - OpenBao server settings
- [Gateway API](gateway-api.md) - HTTPRoute and Gateway integration

## Operations

Day-2 operations and maintenance:

- [Upgrades](upgrades.md) - Rolling and Blue/Green upgrade strategies
- [Backups](backups.md) - Scheduled Raft snapshots to object storage
- [Restore](restore.md) - Restoring from backups
- [Pausing Reconciliation](pausing.md) - Safe manual maintenance
- [Deletion](deletion.md) - DeletionPolicy and cleanup

## Security & Multi-Tenancy

Secure your deployment:

- [Security Profiles](security-profiles.md) - Hardened vs Development profiles
- [Self-Initialization](self-init.md) - Automated cluster bootstrap
- [Sentinel](sentinel.md) - Drift detection and fast-path reconciliation
- [Multi-Tenancy](multi-tenancy.md) - Tenant isolation and governance
- [Tenant Namespaces](tenant-namespaces.md) - Provisioner setup
- [Security Considerations](security-considerations.md) - Best practices
- [Production Checklist](production-checklist.md) - Pre-production verification

## Recovery

Runbooks for common failure scenarios:

- [Break Glass / Safe Mode](recovery-safe-mode.md) - Emergency recovery mode
- [No Leader / No Quorum](recovery-no-leader.md) - Raft quorum recovery
- [Sealed Cluster](recovery-sealed-cluster.md) - Unseal troubleshooting
- [Failed Rollback](recovery-failed-rollback.md) - Blue/Green rollback recovery
- [Restore After Upgrade](recovery-restore-after-upgrade.md) - Partial upgrade recovery

## See Also

- [Architecture](../architecture/index.md) - Deep dive into controller design
- [Security](../security/index.md) - Security model and threat analysis
- [Contributing](../contributing/index.md) - Development and testing
