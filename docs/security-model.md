Here is a draft for a `security-model.md` document that outlines the security architecture of the OpenBao Operator. This document complements the existing Threat Model by focusing on the concrete security controls and implementation details.

***

# Security Model: OpenBao Operator

## 1. Overview

This document details the security model implemented by the OpenBao Operator. It covers the specific controls used to secure the operator itself, the OpenBao clusters it manages, and the boundaries between tenants in a multi-tenant environment.

The security model relies on a **Supervisor Pattern**, where the operator orchestrates security-critical configuration (TLS, unseal keys, network policies) from the outside, while delegating data plane security to OpenBao itself.

## 2. RBAC & Multi-Tenancy

The Operator uses a namespace-scoped RBAC model to support strict tenant isolation. It avoids cluster-wide privileges for tenant operations where possible.

### 2.1 Operator Privileges
The Operator Controller runs with a ClusterRole that allows it to manage resources across the cluster. However, strictly scoped access is enforced for tenants.

* **Controller Permissions:** The operator requires permissions to manage `StatefulSets`, `Services`, `ConfigMaps`, `Secrets`, and `NetworkPolicies`.
* **Blind Writes:** For sensitive assets like the Unseal Key Secret, the operator uses a "blind create" pattern. It attempts to create the Secret but does not require `GET` or `LIST` permissions on the generated secret data after creation, minimizing the attack surface if the operator is compromised.

### 2.2 Tenant Isolation (Namespace Provisioner)
The Operator includes a **Namespace Provisioner** controller that automatically manages RBAC for tenant namespaces labeled with `openbao.org/tenant=true`.

* **Tenant Role (`openbao-operator-tenant-role`):** A Role is created in each tenant namespace granting the operator permission to manage *only* OpenBao-related resources in that specific namespace.
* **RoleBinding:** A RoleBinding ties the Operator's ServiceAccount to this Role, ensuring it only operates on tenant resources within their designated boundary.

**Recommendation:** Tenants should typically be granted `edit` or `view` roles for `OpenBaoCluster` resources but should **not** have `get` access to Secrets matching `*-root-token` or `*-unseal-key`.

## 3. Validating Webhooks & Configuration Security

The Operator enforces a strict "Secure by Default" posture for OpenBao configuration using a Validating Admission Webhook.

### 3.1 Configuration Allowlist
The operator rejects `OpenBaoCluster` resources that attempt to override protected configuration stanzas. This prevents tenants from weakening security controls managed by the operator.

* **Protected Stanzas:** Users cannot override `listener`, `storage`, `seal`, `api_addr`, or `cluster_addr` via `spec.config`. These are strictly owned by the operator to ensure mTLS and Raft integrity.
* **Parameter Allowlist:** Only known-safe OpenBao configuration parameters are accepted in `spec.config`. Unknown or arbitrary keys are rejected.

### 3.2 Immutability
Certain security-critical fields, such as the enabling/disabling of the Init Container, are validated to ensure the cluster cannot be put into an unsupported or insecure state.

## 4. Network Security

The Operator adopts a "Default Deny" network posture for every OpenBao cluster it creates.

### 4.1 Automated NetworkPolicies
For every `OpenBaoCluster`, the operator automatically creates a Kubernetes `NetworkPolicy`.

* **Default Ingress Deny:** All ingress traffic is blocked by default.
* **Allow Rules:**
    * **Inter-Pod Traffic:** Allows traffic between OpenBao pods within the same cluster (required for Raft replication).
    * **Operator Access:** Allows ingress from the OpenBao Operator pods on port 8200 (required for health checks, initialization, and leader step-down operations).
    * **Kube-System:** Allows ingress from `kube-system` for necessary components like DNS.
    * **Gateway API:** If `spec.gateway` is enabled, traffic is allowed from the Gateway's namespace.

### 4.2 Egress Control
Egress is restricted to essential services:
* **DNS:** UDP/TCP on port 53.
* **Kubernetes API:** TCP on port 443/6443 (required for the `discover-k8s` provider to find peer pods).
* **Cluster Peers:** Communication with other Raft peers.

**Note:** Backup jobs run in separate pods that are excluded from this strict policy to allow them to reach external object storage endpoints (S3, GCS, etc.).

## 5. Workload Security

The operator ensures that OpenBao pods run with restricted privileges.

### 5.1 Pod Security Context
The `StatefulSet` creates pods with a hardened security context:

* **Non-Root:** Pods run as user/group 1000 (non-root).
* **Read-Only Root Filesystem:** The root filesystem is mounted read-only to prevent tampering. Configuration and data are written to specific mounted volumes.
* **Capability Drop:** All capabilities are dropped (`ALL`) to minimize privilege escalation risks.
* **Seccomp Profile:** Sets `RuntimeDefault` seccomp profile.
* **No Privilege Escalation:** `AllowPrivilegeEscalation` is set to false.

### 5.2 Init Containers
An init container (`bao-config-init`) is used to render the OpenBao configuration (`config.hcl`) at runtime.
* **Purpose:** It injects dynamic environment variables (like Pod IP and Hostname) into the config template securely, without requiring the main container to run a shell or template engine.
* **Security:** This container runs with the same non-root restrictions as the main container.

## 6. Secret Management

The Operator manages several high-value secrets.

### 6.1 Static Auto-Unseal Key
* **Generation:** A 32-byte cryptographically secure random key is generated by the operator if one does not exist.
* **Storage:** Stored in a Secret named `<cluster>-unseal-key`.
* **Mounting:** Mounted at `/etc/bao/unseal` in the OpenBao pod.
* **Risk:** This key acts as the root of trust for data encryption. The Operator sets a `ConditionEtcdEncryptionWarning` status if it cannot verify that etcd encryption is enabled, warning admins that physical access to etcd could compromise this key.

### 6.2 Root Token
* **Lifecycle:** During manual bootstrap (non-self-init), the initial root token is stored in `<cluster>-root-token`.
* **Recommendation:** Users are strongly advised to revoke this token or delete the Secret immediately after initial setup.
* **Self-Init:** When `spec.selfInit` is used, the root token is automatically revoked by OpenBao after initialization and is **never** stored in a Secret.

## 7. TLS & Identity

The Operator acts as an internal Certificate Authority (CA) to enforce mTLS.

* **Automated PKI:** The operator generates a self-signed Root CA and issues ephemeral leaf certificates for every cluster.
* **Strict SANs:** Certificates include strict Subject Alternative Names (SANs) for the Service and Pod DNS names. Pod IPs are explicitly *excluded* to avoid identity fragility during pod churn.
* **Rotation:** Server certificates are automatically rotated before expiry (configurable via `spec.tls.rotationPeriod`).
* **Gateway Integration:** When Gateway API is used, the operator manages a CA ConfigMap to allow the Gateway (e.g., Traefik) to validate the backend OpenBao pods, ensuring full end-to-end TLS.