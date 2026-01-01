# OpenBaoTenant

`OpenBaoTenant` is the governance/onboarding CRD that authorizes the operator to manage OpenBao resources in a namespace by provisioning tenant-scoped RBAC.

- Use this when you want to onboard a namespace for `OpenBaoCluster` resources.
- In self-service mode, a tenant can only target its own namespace (`spec.targetNamespace == metadata.namespace`).

Next:

- [Tenant Onboarding](onboarding.md)
- [Multi-Tenancy](multi-tenancy.md)
