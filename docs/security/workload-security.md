# Workload Security

The operator ensures that OpenBao pods run with restricted privileges.

## Pod Security Context

The `StatefulSet` creates pods with a hardened security context:

| Setting | Value | Purpose |
| ------- | ----- | ------- |
| Run As User/Group | 1000 | Non-root execution |
| Read-Only Root FS | `true` | Prevents filesystem tampering |
| Capabilities | `ALL` dropped | Minimizes privilege escalation risks |
| Seccomp Profile | `RuntimeDefault` | Restricts syscalls |
| Allow Privilege Escalation | `false` | Prevents privilege escalation |

Configuration and data are written to specific mounted volumes, not the root filesystem.

## ServiceAccount Token Handling

The Operator disables Pod-level ServiceAccount token automounting and instead mounts a short-lived projected token only into the OpenBao container. This reduces token exposure while still enabling:

- Kubernetes auto-join discovery (`retry_join` via `discover-k8s`).
- Kubernetes service registration (OpenBao-managed Pod labels such as `openbao-active`, `openbao-initialized`, and `openbao-sealed`).

## Init Containers

An init container (`bao-config-init`) is used to render the OpenBao configuration (`config.hcl`) at runtime.

- **Purpose:** It injects dynamic environment variables (like Pod IP and Hostname) into the config template securely, without requiring the main container to run a shell or template engine.
- **Security:** This container runs with the same non-root restrictions as the main container.

## Pod Security Standards

The Provisioner automatically applies Pod Security Standards (PSS) labels to tenant namespaces:

| Label | Value |
| ----- | ----- |
| `pod-security.kubernetes.io/enforce` | `restricted` |
| `pod-security.kubernetes.io/audit` | `restricted` |
| `pod-security.kubernetes.io/warn` | `restricted` |

**Security Benefits:**

- Enforces Restricted PSS compliance for all workloads in tenant namespaces.
- Prevents deployment of workloads that do not meet security requirements.
- Provides consistent security posture across all tenant namespaces.
- Aligns with Kubernetes security best practices.
