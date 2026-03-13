<div align="center">

<img src="docs/assets/repo_logo.png" alt="OpenBao Operator" width="520" />


**Secure lifecycle management for OpenBao on Kubernetes.**

[![CI](https://github.com/dc-tec/openbao-operator/actions/workflows/ci.yml/badge.svg)](https://github.com/dc-tec/openbao-operator/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/dc-tec/openbao-operator?filename=go.mod&label=Go&logo=go&logoColor=white)](https://github.com/dc-tec/openbao-operator/blob/main/go.mod)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Docs](https://img.shields.io/badge/Docs-Live-green)](https://dc-tec.github.io/openbao-operator/)
[![Artifact Hub](https://img.shields.io/badge/Artifact_Hub-Helm_OCI-417598?logo=artifacthub&logoColor=white)](https://artifacthub.io/packages/search?repo=openbao-operator)

[Quick Start](#quick-start) • [Installation](#installation) • [Compatibility](#compatibility) • [Documentation](#documentation) • [Contributing](#contributing)

</div>

> [!WARNING]
> **Pre-GA Release**: OpenBao Operator is intended for real deployments, but the CRD API remains `v1alpha1`, minor releases may introduce breaking changes, and support is best-effort for the latest stable line only. For production, use the `Hardened` profile, keep admission enforcement enabled, pin explicit versions, and validate upgrades in staging.

---

OpenBao Operator is a Kubernetes operator for [OpenBao](https://openbao.org) that automates lifecycle management: provisioning, TLS, backups/restores, upgrades, and multi-tenancy controls.

## Documentation

Full documentation is available at **[dc-tec.github.io/openbao-operator](https://dc-tec.github.io/openbao-operator/)**.

| | |
| :---: | :---: |
| [![User Guide](https://img.shields.io/badge/User_Guide-007EC6?style=for-the-badge&logo=readthedocs&logoColor=white)](https://dc-tec.github.io/openbao-operator/latest/user-guide/) | [![Architecture](https://img.shields.io/badge/Architecture-326CE5?style=for-the-badge&logo=kubernetes&logoColor=white)](https://dc-tec.github.io/openbao-operator/latest/architecture/) |
| **Installation, Operations, Day-2 Tasks** | **Component Design, Boundaries, Flows** |
| [![Security](https://img.shields.io/badge/Security-000000?style=for-the-badge&logo=imou&logoColor=white)](https://dc-tec.github.io/openbao-operator/latest/security/) | [![Contributing](https://img.shields.io/badge/Contributing-181717?style=for-the-badge&logo=github&logoColor=white)](https://dc-tec.github.io/openbao-operator/latest/contributing/) |
| **Threat Model, Hardening, RBAC** | **Dev Setup, Coding Standards, Release** |
| [![Compatibility](https://img.shields.io/badge/Compatibility-10b981?style=for-the-badge&logo=kubernetes&logoColor=white)](https://dc-tec.github.io/openbao-operator/latest/reference/compatibility/) | [![Samples](https://img.shields.io/badge/Samples-9333ea?style=for-the-badge&logo=yaml&logoColor=white)](config/samples/) |
| **Validated K8s/OpenBao Versions** | **Ready-to-apply Example Manifests** |

Recommended entry points:

- [Deployment Decision Guide](https://dc-tec.github.io/openbao-operator/latest/user-guide/deployment-decision-guide/)
- [Operator Invariants](https://dc-tec.github.io/openbao-operator/latest/architecture/operator-invariants/)
- [Production Checklist](https://dc-tec.github.io/openbao-operator/latest/user-guide/openbaocluster/operations/production-checklist/)

## Compatibility

For full details, see the [Compatibility Matrix](https://dc-tec.github.io/openbao-operator/latest/reference/compatibility/).

- **Kubernetes**: validated in CI on `v1.33`–`v1.35`
- **OpenBao**: validated in CI on `2.5.0`, with config compatibility checks for `2.4.4` and upgrade coverage from `2.4.4` to `2.5.0`
- **Support posture**: best-effort support for the latest stable release line

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
- **Safe Upgrades**: Rolling and blue/green upgrade strategies, including pre-upgrade snapshots. `RollingUpdate` is the default recommended strategy.
- **Multi-Tenancy**: Namespace-scoped tenancy model with policy enforcement via `OpenBaoTenant`. Multi-tenant mode is the default and recommended production operating model.

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

The default production path is:

- Multi-tenant mode
- `Hardened` profile
- `spec.selfInit.enabled: true`
- `spec.tls.mode: External` or `ACME`
- `spec.upgrade.strategy: RollingUpdate`
- Admission policies enabled

The `Hardened` profile enforces:
- External/ACME TLS (`spec.tls.mode`)
- External unseal (`spec.unseal.type`)
- Self-init enabled (`spec.selfInit.enabled: true`)

Start with:
- [Deployment Decision Guide](https://dc-tec.github.io/openbao-operator/latest/user-guide/deployment-decision-guide/)
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

If you install the operator into a custom namespace, replace `openbao-operator-system` consistently in the install, verification, and uninstall commands.

Find the chart in Artifact Hub (indexing may lag shortly after releases):
[Artifact Hub search: openbao-operator](https://artifacthub.io/packages/search?repo=openbao-operator)

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
- Running the PR-equivalent local gate (`make bootstrap && make doctor && make ci-core`).
- Our AI-Assisted Contribution Policy.

## Official OpenBao Documentation

- [OpenBao Documentation](https://openbao.org/docs/)
- [OpenBao Upgrade Guide](https://openbao.org/docs/upgrading/)

## License

Apache-2.0. See `LICENSE`.
