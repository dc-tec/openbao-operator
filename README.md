# OpenBao Operator

**This is an experimental Operator**


[![CI](https://github.com/dc-tec/openbao-operator/actions/workflows/ci.yml/badge.svg)](https://github.com/dc-tec/openbao-operator/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/dc-tec/openbao-operator)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

The OpenBao Operator manages the lifecycle of [OpenBao](https://openbao.org) clusters on Kubernetes.
It follows a Supervisor Pattern: OpenBao owns data consistency (Raft), the operator owns orchestration (PKI, backups, upgrades, multi-tenancy, and hardening).

## Documentation

Docs site: https://dc-tec.github.io/openbao-operator/

| Guide | Description |
|-------|-------------|
| [User Guide](docs/user-guide/index.md) | Installation, configuration, operations, and day-2 tasks |
| [Architecture](docs/architecture/index.md) | Component design, boundaries, lifecycle flows |
| [Security](docs/security/index.md) | Threat model, RBAC, admission policies, hardening |
| [Compatibility](docs/reference/compatibility.md) | Supported Kubernetes and OpenBao versions |
| [Contributing](docs/contributing/index.md) | Development setup, CI, release management |

## Features

- Zero trust architecture: split provisioner/controller with explicit, least-privilege permissions
- Automated PKI: operator-managed CA, cert rotation, safe reloads
- Raft-aware upgrades: rolling and blue/green flows with safety checks
- Backups and restore: streaming Raft snapshots to object storage
- Multi-tenancy: namespace-scoped isolation with RBAC + admission controls
- Supply chain security: signature verification and digest-pinning for operator-managed images

## Installation

Prerequisites: Kubernetes v1.33+ (see `docs/reference/compatibility.md`), `kubectl`, and (recommended) `helm`.

Default namespace: `openbao-operator-system`.

### Install via Helm (recommended)

```sh
kubectl create namespace openbao-operator-system

helm install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
  --version <chart-version> \
  --namespace openbao-operator-system
```

To pin the operator image by digest:

```sh
helm install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
  --version <chart-version> \
  --namespace openbao-operator-system \
  --set image.repository=ghcr.io/dc-tec/openbao-operator \
  --set image.digest=sha256:<digest>
```

CRD upgrades: when upgrading with Helm, apply updated CRDs first, then `helm upgrade`. See `docs/user-guide/operator/installation.md`.

### Install via release `install.yaml`

Download `install.yaml` from the GitHub Release and apply it:

```sh
kubectl apply -f install.yaml
```

### Developer install (Kustomize)

For local development only:

```sh
make install
make deploy IMG=ghcr.io/dc-tec/openbao-operator:dev
```

## Quick start (multi-tenant mode)

### 1) Onboard a tenant namespace

Create a tenant that maps to a target namespace. The Provisioner will create the namespace (if needed) and install tenant-scoped RBAC.

```sh
kubectl apply -f - <<'YAML'
apiVersion: openbao.org/v1alpha1
kind: OpenBaoTenant
metadata:
  name: my-tenant
  namespace: openbao-operator-system
spec:
  targetNamespace: my-namespace
YAML
```

### 2) Create an OpenBao cluster

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: my-cluster
  namespace: my-namespace
spec:
  version: "2.4.4"
  image: "openbao/openbao:2.4.4"
  replicas: 3
  profile: Development # use Hardened for production
  tls:
    enabled: true
    mode: OperatorManaged
  storage:
    size: "10Gi"
```

```sh
kubectl apply -f cluster.yaml
kubectl -n my-namespace get pods -l openbao.org/cluster=my-cluster -w
```

Production note: the example above uses `Development`. For production, use `profile: Hardened` and follow `docs/user-guide/openbaocluster/operations/production-checklist.md`.

## Helm chart maintenance

The chart includes CRDs and a templated installer manifest. CI enforces that these stay in sync:

- Sync chart inputs: `make helm-sync`
- Verify (PR-equivalent): `make verify-helm`

See `docs/contributing/ci.md`.

## Architecture

The operator is split into two components to enforce security boundaries:

```
┌─────────────────────────────────────────────────────────────────┐
│                      Kubernetes API Server                      │
└─────────────────────────────────────────────────────────────────┘
         ▲                                    ▲
         │ watch Tenants                      │ watch Clusters
         │                                    │
┌────────┴────────┐                ┌──────────┴──────────┐
│   Provisioner   │                │    Controller       │
│ (Cluster-scoped)│                │ (Namespace-scoped)  │
│                 │                │                     │
│ • Creates NS    │                │ • StatefulSet       │
│ • Grants RBAC   │                │ • ConfigMaps        │
│ • Tenant CRs    │                │ • Secrets (TLS)     │
└─────────────────┘                │ • Backups           │
                                   │ • Upgrades          │
                                   └─────────────────────┘
```

For detailed architecture diagrams and component interactions, see [Architecture Documentation](docs/architecture/index.md).

## Contributing

We welcome contributions! Before submitting:

1. Follow the coding standards in [Contributing Guide](docs/contributing/index.md)
2. Ensure `make test` and `make lint` pass locally (or run `make verify-fmt verify-tidy verify-generated verify-helm test-ci`)
3. Keep documentation in sync with behavior changes

### AI-Assisted Contributions

We welcome contributions that leverage AI tools. However, all contributions, AI-assisted or not must meet our quality standards:

- **Understand what you're submitting.** You are responsible for the code you contribute.
- **Follow the coding standards** in our [Contributing Guide](docs/contributing/index.md).
- **Test your changes.** PRs must pass CI and include appropriate test coverage.
- **Write meaningful commit messages** that explain the "why," not just the "what."

PRs that appear to be low-effort AI-generated content without proper review, testing, or understanding will be closed without merge.

> **Tip for AI users**: Configure your AI tool to use `.agent/rules/` as workspace rules for project-specific standards.

## License

Apache-2.0. See `LICENSE`.
