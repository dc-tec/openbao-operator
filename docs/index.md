---
description: OpenBao Operator is a Kubernetes operator for secure OpenBao lifecycle management, including installs, upgrades, backup and restore, and multi-tenant operations.
---

# OpenBao Operator

OpenBao Operator is a Kubernetes operator for [OpenBao](https://openbao.org), focused on secure lifecycle management: install, upgrades, backup/restore, and multi-tenant operations.

!!! note "Pre-GA Release"
    OpenBao Operator is pre-GA and intended for real deployments, but the CRD API remains `v1alpha1`, minor releases may introduce breaking changes, and support is best-effort for the latest stable line only. For production, use the `Hardened` profile, keep admission enforcement enabled, pin explicit versions, and validate upgrades in staging.

<div class="grid cards" markdown>

- :material-book-open-page-variant: **User Guide**

    ---

    Step-by-step guides to deploy, configure, and operate OpenBao clusters on Kubernetes.

    [:material-arrow-right: Getting Started](user-guide/index.md)

- :material-shield-lock: **Security**

    ---

    Threat modeling, RBAC design, admission policies, and security hardening guidelines.

    [:material-arrow-right: Explore Security](security/index.md)

- :material-server-network: **Architecture**

    ---

    Deep dive into the controller design, reconciliation loops, and key lifecycle flows.

    [:material-arrow-right: View Ecosystem](architecture/index.md)

- :material-code-braces: **Contributing**

    ---

    Setup your development environment, build targets, and testing strategies.

    [:material-arrow-right: Start Contributing](contributing/index.md)

</div>

## Why OpenBao Operator?

<div class="grid cards" markdown>

- :material-autorenew: **Automated Lifecycle**

    ---

    Seamlessly provision, scale, and upgrade clusters with zero downtime using advanced state management.

- :material-security: **Security First**

    ---

    Secure-by-default configuration with automated TLS rotation, sealed unsealing, and strict RBAC profiles.

- :material-database-clock: **Day 2 Operations**

    ---

    Built-in backup/restore controllers, automated snapshots, and detailed metrics for production reliability.

- :material-kubernetes: **Kubernetes Native**

    ---

    Designed with standard CRDs, detailed Status conditions, and full integration with the Kubernetes ecosystem.

</div>

## Community

Connect with other users and contributors:

<div class="grid cards" markdown>

- :simple-github: **GitHub**

    ---

    Report bugs, request features, and contribute code.

    [:material-arrow-right: Go to Repository](https://github.com/dc-tec/openbao-operator)

- :material-package-variant-closed: **Artifact Hub**

    ---

    Discover the Helm OCI package page and installation metadata.

    [:material-arrow-right: Search Package](https://artifacthub.io/packages/search?repo=openbao-operator)

</div>

## Reference

- [**API Reference**](reference/api.md) — Generated CRD schema and field-level validation/default details.
- [**Compatibility Matrix**](reference/compatibility.md) — Supported Kubernetes and OpenBao versions.
- [**Deprecation Policy**](reference/deprecation-policy.md) — API lifecycle and breaking-change rules for `0.x`.
- [**Support & Maintenance**](reference/support-policy.md) — Supported channels, release window, and support expectations.
- [**Operator Upgrade Compatibility**](reference/operator-upgrade-compatibility.md) — Supported operator upgrade paths and CRD upgrade order.
- [**Status Conditions & Events**](reference/status-and-events.md) — Condition types, reasons, and common events for operations.
- [**Known Limitations**](reference/known-limitations.md) — Current non-goals and known constraints.

## Official OpenBao Documentation

- [OpenBao Documentation](https://openbao.org/docs/)
- [OpenBao Upgrade Guide](https://openbao.org/docs/upgrading/)
