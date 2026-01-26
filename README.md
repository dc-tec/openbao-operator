<div align="center">

# OpenBao Operator

**Enterprise-grade management for OpenBao on Kubernetes.**

[![CI](https://github.com/dc-tec/openbao-operator/actions/workflows/ci.yml/badge.svg)](https://github.com/dc-tec/openbao-operator/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.25.5-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Docs](https://img.shields.io/badge/Docs-Live-green)](https://dc-tec.github.io/openbao-operator/)

[Quick Start](#quick-start) • [Installation](#installation) • [Compatibility](#compatibility) • [Documentation](#documentation) • [Contributing](#contributing)

</div>

> [!WARNING]
> **Pre-1.0 Status**: This operator is actively seeking feedback and may introduce breaking changes to APIs and defaults before `v1.0.0`. Validate thoroughly before production use.

---

The OpenBao Operator manages the lifecycle of [OpenBao](https://openbao.org) clusters on Kubernetes using a **Supervisor Pattern**. It handles the orchestration complexity—PKI, backups, upgrades, and secure multi-tenancy—so you can focus on consuming secrets.

## Documentation

Full documentation is available at **[dc-tec.github.io/openbao-operator](https://dc-tec.github.io/openbao-operator/)**.

| | |
| :---: | :---: |
| [![User Guide](https://img.shields.io/badge/User_Guide-007EC6?style=for-the-badge&logo=readthedocs&logoColor=white)](https://dc-tec.github.io/openbao-operator/latest/user-guide/) | [![Architecture](https://img.shields.io/badge/Architecture-326CE5?style=for-the-badge&logo=kubernetes&logoColor=white)](https://dc-tec.github.io/openbao-operator/latest/architecture/) |
| **Installation, Operations, Day-2 Tasks** | **Component Design, Boundaries, Flows** |
| [![Security](https://img.shields.io/badge/Security-000000?style=for-the-badge&logo=imou&logoColor=white)](https://dc-tec.github.io/openbao-operator/latest/security/) | [![Contributing](https://img.shields.io/badge/Contributing-181717?style=for-the-badge&logo=github&logoColor=white)](https://dc-tec.github.io/openbao-operator/latest/contributing/) |
| **Threat Model, Hardening, RBAC** | **Dev Setup, Coding Standards, Release** |
| [![Compatibility](https://img.shields.io/badge/Compatibility-10b981?style=for-the-badge&logo=kubernetes&logoColor=white)](https://dc-tec.github.io/openbao-operator/latest/reference/compatibility/) | [![Samples](https://img.shields.io/badge/Samples-9333ea?style=for-the-badge&logo=yaml&logoColor=white)](config/samples/) |
| **Supported K8s/OpenBao Versions** | **Ready-to-apply Example Manifests** |

## Compatibility

For full details, see the [Compatibility Matrix](https://dc-tec.github.io/openbao-operator/latest/reference/compatibility/).

- **Kubernetes**: `v1.33+` (tested: `v1.33`–`v1.35`)
- **OpenBao**: >= `2.4.x`

## CRDs (API Surface)

- `OpenBaoCluster`: Deploy and operate an OpenBao cluster (TLS, unseal, backups, upgrades).
- `OpenBaoRestore`: Restore a cluster from a backup (separate controller).
- `OpenBaoTenant`: Multi-tenant provisioning flow (multi-tenant mode).

## Features

- **Two-Controller Architecture**: Separate controller and provisioner components with least-privilege RBAC boundaries.
- **Security Profiles with Guardrails**: `Development` vs `Hardened`, enforced by admission policies to prevent insecure combinations.
- **Self-Init + OIDC Bootstrap**: OpenBao self-initialization, with optional JWT/OIDC bootstrap via `spec.selfInit.oidc.enabled`.
- **TLS, Your Way**: Operator-managed TLS with rotation, external TLS, and ACME mode where OpenBao owns certificates (with ACME challenge Service support).
- **Streaming Raft Backups**: Snapshot streaming to S3/GCS/Azure with retention controls (no local staging).
- **Declarative Restores**: Restore workflows via `OpenBaoRestore` with operation locking and safe overrides.
- **Safe Upgrades**: Rolling and blue/green upgrade strategies, including pre-upgrade snapshots.
- **Multi-Tenancy**: Namespace-scoped tenancy model with policy enforcement via `OpenBaoTenant`.

## Security Model

- **Threat model**: Design assumptions and attacker model ([Threat Model](https://dc-tec.github.io/openbao-operator/latest/security/fundamentals/threat-model/))
- **RBAC boundaries**: Least-privilege split between controller and provisioner ([RBAC](https://dc-tec.github.io/openbao-operator/latest/security/infrastructure/rbac/))
- **Guardrails**: Validating admission policies that block dangerous settings before they reach the cluster ([Admission Policies](https://dc-tec.github.io/openbao-operator/latest/security/infrastructure/admission-policies/))
- **Multi-tenancy**: Namespace isolation guarantees and limits ([Tenant Isolation](https://dc-tec.github.io/openbao-operator/latest/security/multi-tenancy/tenant-isolation/))

## Quick Start

Once the operator is running, you can launch an OpenBao cluster quickly.

### Option A: Evaluation (Development Profile)

```yaml
# cluster.yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: my-cluster
  namespace: openbao-demo
spec:
  version: "2.4.4"
  replicas: 1
  profile: Development
  tls:
    enabled: true
    mode: OperatorManaged
  storage:
    size: "10Gi"
```

```bash
kubectl create namespace openbao-demo
kubectl apply -f cluster.yaml

# Watch status and pods
kubectl -n openbao-demo get openbaoclusters my-cluster -w
kubectl -n openbao-demo get pods -l openbao.org/cluster=my-cluster -w
```

If `spec.selfInit.enabled` is `false` (default), the operator stores a root token in `Secret/openbao-demo/my-cluster-root-token` (key: `token`).

```bash
kubectl -n openbao-demo get secret my-cluster-root-token -o jsonpath='{.data.token}' | base64 -d; echo
```

### Option B: Production (Hardened Profile)

The `Hardened` profile is the recommended production posture and enforces:
- External/ACME TLS (`spec.tls.mode`)
- External unseal (`spec.unseal.type`)
- Self-init enabled (`spec.selfInit.enabled: true`)

Start with:
- [Security Profiles](https://dc-tec.github.io/openbao-operator/latest/user-guide/openbaocluster/configuration/security-profiles/)
- [Production Checklist](https://dc-tec.github.io/openbao-operator/latest/user-guide/openbaocluster/operations/production-checklist/)
- Production samples in `config/samples/production/`

## Installation

### Option 1: Helm (Recommended)

Install the operator from our OCI registry.

```bash
# 1. Create namespace
kubectl create namespace openbao-operator-system

# 2. Install/upgrade chart
helm upgrade --install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
  --version <chart-version> \
  --namespace openbao-operator-system
```

### Option 2: Plain YAML

Apply the latest release manifest directly.

```bash
kubectl apply -f https://github.com/dc-tec/openbao-operator/releases/latest/download/install.yaml
```

## Uninstall

### Helm

```bash
helm uninstall openbao-operator --namespace openbao-operator-system
```

### Plain YAML

```bash
kubectl delete -f https://github.com/dc-tec/openbao-operator/releases/latest/download/install.yaml
```

> [!NOTE]
> The operator installation includes CRDs. If you want to remove CRDs as well, delete the `openbao.org/*` CRDs after uninstalling (this will delete all custom resources).

## Contributing

We welcome contributions! Please see the [Contributing Guide](https://dc-tec.github.io/openbao-operator/latest/contributing/) for details on:

- Setting up your development environment.
- Running tests (`make test-ci`).
- Our AI-Assisted Contribution Policy.

## License

Apache-2.0. See `LICENSE`.
