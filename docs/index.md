# OpenBao Operator

Welcome to the documentation for the OpenBao Operator, a Kubernetes native operator for managing [OpenBao](https://openbao.org) clusters.

!!! warning "Experimental Status"
    This operator is currently in an **experimental phase** and is actively seeking feedback. It is **not** recommended for production environments at this time.

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

</div>

## Reference

- [**Compatibility Matrix**](reference/compatibility.md) — Supported Kubernetes and OpenBao versions.
