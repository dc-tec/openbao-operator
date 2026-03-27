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
| [![User Guide](https://img.shields.io/badge/User_Guide-007EC6?style=for-the-badge&logo=readthedocs&logoColor=white)](https://dc-tec.github.io/openbao-operator/docs/get-started) | [![Architecture](https://img.shields.io/badge/Architecture-326CE5?style=for-the-badge&logo=kubernetes&logoColor=white)](https://dc-tec.github.io/openbao-operator/docs/architecture) |
| **Installation, Operations, Day-2 Tasks** | **Component Design, Boundaries, Flows** |
| [![Security](https://img.shields.io/badge/Security-000000?style=for-the-badge&logo=imou&logoColor=white)](https://dc-tec.github.io/openbao-operator/docs/security) | [![Contributing](https://img.shields.io/badge/Contributing-181717?style=for-the-badge&logo=github&logoColor=white)](https://dc-tec.github.io/openbao-operator/contribute) |
| **Threat Model, Hardening, RBAC** | **Dev Setup, Coding Standards, Release** |
| [![Compatibility](https://img.shields.io/badge/Compatibility-10b981?style=for-the-badge&logo=kubernetes&logoColor=white)](https://dc-tec.github.io/openbao-operator/docs/reference/compatibility) | [![Samples](https://img.shields.io/badge/Samples-9333ea?style=for-the-badge&logo=yaml&logoColor=white)](config/samples/) |
| **Validated K8s/OpenBao Versions** | **Ready-to-use YAML Examples** |

## Quick Start

Deploy the operator and a development OpenBao instance in minutes:

```bash
# 1. Install the Operator via Helm
helm install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator --namespace openbao-system --create-namespace

# 2. Deploy a sample OpenBao cluster
kubectl apply -f config/samples/openbao_v1alpha1_openbao.yaml
```

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.