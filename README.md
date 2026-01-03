<div align="center">

# OpenBao Operator

**Enterprise-grade management for OpenBao on Kubernetes.**

[![CI](https://github.com/dc-tec/openbao-operator/actions/workflows/ci.yml/badge.svg)](https://github.com/dc-tec/openbao-operator/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Docs](https://img.shields.io/badge/Docs-Live-green)](https://dc-tec.github.io/openbao-operator/)

[Features](#features) • [Installation](#installation) • [Documentation](#documentation) • [Contributing](#contributing)

</div>

> [!WARNING]
> **Experimental Status**: This operator is currently in an experimental phase and is actively seeking feedback. It is **not** recommended for production environments at this time.

---

The OpenBao Operator manages the lifecycle of [OpenBao](https://openbao.org) clusters on Kubernetes using a **Supervisor Pattern**. It handles the orchestration complexity—PKI, backups, upgrades, and secure multi-tenancy—so you can focus on consuming secrets.

## Documentation

Full documentation is available at **[dc-tec.github.io/openbao-operator](https://dc-tec.github.io/openbao-operator/)**.

| | |
| :---: | :---: |
| [![User Guide](https://img.shields.io/badge/User_Guide-007EC6?style=for-the-badge&logo=readthedocs&logoColor=white)](docs/user-guide/index.md) | [![Architecture](https://img.shields.io/badge/Architecture-326CE5?style=for-the-badge&logo=kubernetes&logoColor=white)](docs/architecture/index.md) |
| **Installation, Operations, Day-2 Tasks** | **Component Design, Boundaries, Flows** |
| [![Security](https://img.shields.io/badge/Security-000000?style=for-the-badge&logo=imou&logoColor=white)](docs/security/index.md) | [![Contributing](https://img.shields.io/badge/Contributing-181717?style=for-the-badge&logo=github&logoColor=white)](docs/contributing/index.md) |
| **Threat Model, Hardening, RBAC** | **Dev Setup, Coding Standards, Release** |

## Features

- **Zero Trust Architecture**: Dedicated strict RBAC for the Provisioner (cluster-scoped) vs Controller (namespace-scoped).
- **Automated PKI**: Built-in Certificate Authority that handles rotation and hot reloads for all TLS traffic.
- **Raft Streaming**: Native backups that stream Raft snapshots directly to S3/GCS/Azure without local disk staging.
- **Safe Upgrades**: Automated Rolling and Blue/Green upgrade strategies with Raft health checks.
- **Multi-Tenancy**: Securely share a cluster with namespace isolation and policy enforcement.

## Installation

### Option 1: Helm (Recommended)

Install the operator from our OCI registry.

```bash
# 1. Create namespace
kubectl create namespace openbao-operator-system

# 2. Install Chart
helm install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
  --version 0.1.0 \
  --namespace openbao-operator-system
```

### Option 2: Plain YAML

Apply the latest release manifest directly.

```bash
kubectl apply -f https://github.com/dc-tec/openbao-operator/releases/latest/download/install.yaml
```

## Quick Start: Launch a Cluster

Once the operator is running, you can launch a production-ready OpenBao cluster in seconds.

```yaml
# cluster.yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: my-cluster
  namespace: my-namespace
spec:
  version: "2.4.4"
  image: "openbao/openbao:2.4.4"
  replicas: 3
  profile: Development 
  tls:
    enabled: true
    mode: OperatorManaged
  storage:
    size: "10Gi"
```

```bash
kubectl apply -f cluster.yaml
```

> **Note**: For production, verify prerequisites in the [Production Checklist](docs/user-guide/openbaocluster/operations/production-checklist.md).

## Contributing

We welcome contributions! Please see the [Contributing Guide](docs/contributing/index.md) for details on:

- Setting up your development environment.
- Running tests (`make test-ci`).
- Our AI-Assisted Contribution Policy.

## License

Apache-2.0. See `LICENSE`.
