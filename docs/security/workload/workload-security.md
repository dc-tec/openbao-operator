# Workload Security

!!! abstract "Core Concept"
    The Operator ensures that OpenBao pods run with **Restricted Privileges** by default. This minimizes the blast radius of a container escape and enforces isolation at the runtime level.

## Pod Security Context

The `StatefulSet` creates pods with a hardened security context compliant with the **Restricted** Pod Security Standard.

| Setting | Value | Purpose |
| :--- | :--- | :--- |
| **Run As User/Group** | Default `100:1000` (Kubernetes mode) | Ensures non-root execution with a stable UID/GID baseline. |
| **Read-Only Root FS** | `true` | Prevents filesystem tampering and immutable infrastructure violations. |
| **Capabilities** | `ALL` dropped | Minimizes privilege escalation risks. |
| **Seccomp Profile** | `RuntimeDefault` | Restricts available syscalls to the kernel. |
| **Privilege Escalation** | `false` | Prevents setuid binaries from gaining root. |

!!! note "Volume Mounts"
    Since the root filesystem is read-only, all mutable data (logs, storage, tmp) is written to explicit writable volume mounts. Backup and restore jobs also run with explicit resource requests and limits.

## Resource Guardrails

The Operator places default resource limits on ephemeral jobs to protect the node from "noisy neighbor" resource exhaustion.

!!! tip "Default Job Limits"
    These defaults ensure that a stuck backup job or an aggressive snapshot process doesn't starve the actual OpenBao pods (or other tenants) on the same node.

| Job Type | Resource | Request | Limit |
| :--- | :--- | :--- | :--- |
| **Backup / Restore** | CPU | `100m` | `500m` |
| | Memory | `128Mi` | `512Mi` |

## ServiceAccount Token Handling

The Operator minimizes the attack surface of the Kubernetes JWT token:

1. **No Automounting:** `automountServiceAccountToken: false` is set on the Pod spec.
2. **Projected Volume:** A short-lived projected token is mounted *only* into the OpenBao container (not init containers). If needed, an explicit audience can be configured.

**Token Usage:**

- **Peering:** Used by `discover-k8s` to find other Raft peers.
- **Registration:** Used to update Pod labels (`openbao-active`, `openbao-sealed`) for service handling.

## Init Containers

An init container (`bao-config-init`) is used to render the OpenBao configuration (`config.hcl`) at runtime.

- **Purpose:** Injects dynamic environment variables (Pod IP, Hostname) into the config template securely.
- **Security:** Runs under the Pod-level non-root security context (including capability drop, read-only root filesystem, and `runAsNonRoot=true`) and does not receive the Kubernetes API token mount.

!!! note "Platform and Override Behavior"
    On OpenShift, UID/GID values are typically assigned by SCC instead of being pinned.
    `spec.securityContext` can also override default IDs on standard Kubernetes clusters.

## Pod Security Standards (PSS)

The Provisioner automatically applies PSS labels to any Tenant namespace it creates:

| Label | Value | Enforcement |
| :--- | :--- | :--- |
| `pod-security.kubernetes.io/enforce` | `restricted` | **Hard Block** |
| `pod-security.kubernetes.io/audit` | `restricted` | **Audit Log** |
| `pod-security.kubernetes.io/warn` | `restricted` | **User Warning** |

**Impact:**
Any workload deployed into a Tenant namespace (by the user or operator) MUST meet these strict standards or the API server will reject it. This prevents users from accidentally deploying insecure "sidecar" workloads alongside OpenBao.
