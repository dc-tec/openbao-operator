# OpenBao Supervisor Operator

The OpenBao Supervisor Operator manages the full lifecycle of OpenBao clusters on Kubernetes. It adopts a **Supervisor Pattern**, delegating data consistency to OpenBao (Raft) while managing the external ecosystem (PKI, Upgrades, Backups).

## Architecture Overview
```mermaid
flowchart LR
  subgraph Tenants["User / Tenant Namespaces"]
    U["Users / Platform Teams"]
    CR["OpenBaoCluster CR<br/>openbao.org/v1alpha1"]
  end

  subgraph K8sAPI["Kubernetes API Server"]
    API["apiserver"]
  end

  subgraph OperatorNS["Operator Namespace"]
    CM["Controller Manager<br/>(controller-runtime)"]

    subgraph Reconcilers["Reconcilers / Managers"]
      OC["OpenBaoCluster Reconciler"]
      Cert["Cert Manager<br/>internal/certs"]
      Infra["Infra / Config Manager<br/>internal/infra + internal/config"]
      Upg["Upgrade Manager<br/>internal/upgrade"]
      Bkp["Backup Manager<br/>internal/backup"]
    end
  end

  subgraph ClusterNS["OpenBao Cluster Namespace"]
    STS["OpenBao StatefulSet"]
    Pods["OpenBao Pods"]
    PVC["PersistentVolumes / PVCs"]
    TLS["TLS Secrets<br/>(CA + server certs)"]
    Unseal["Unseal Key Secret"]
    CFG["ConfigMap: config.hcl"]
    Svc["Services (headless + client)"]
    Ing["Ingress (optional)"]
    HTTPRoute["HTTPRoute (optional, Gateway API)"]
    GW["Gateway (user-managed, optional)"]
  end

  Obj["Object Storage<br/>(S3 / GCS / Azure Blob)"]

  %% Relationships
  U -->|create / update| CR
  U -->|provision| GW
  CR -->|stored in| API

  CM -->|watches| API
  CM --> OC

  OC --> Cert
  OC --> Infra
  OC --> Upg
  OC --> Bkp

  Cert -->|generate & rotate| TLS
  Infra -->|manage| Unseal
  Infra --> CFG
  Infra --> Svc
  Infra --> Ing
   Infra --> HTTPRoute
  Infra --> STS

  STS --> Pods
  Pods --> PVC

  GW --> HTTPRoute
  HTTPRoute --> Svc

  Bkp -->|stream snapshots| Obj

  Pods -->|serve OpenBao API<br/>mTLS, Raft| U

```

## Key Features

  * **Automated PKI:** Manages internal Root CA, rotates certificates, and triggers hot-reloads.
  * **Raft-Aware Upgrades:** Orchestrates safe, step-down based rolling upgrades.
  * **Disaster Recovery:** Streams snapshots directly to S3/GCS/Azure without disk buffering.
  * **Secure by Default:** Rootless, read-only filesystem, strict NetworkPolicies, and auto-unseal.
  * **Multi-Tenant:** Namespace-scoped isolation with strict RBAC boundaries.

## Quick Start

**Prerequisites**: Kubernetes v1.25+, kubectl, helm (optional).

### 1. Install the Operator

```sh
make install
make deploy IMG=ghcr.io/openbao/openbao-operator:latest
```

### 2. Deploy a Cluster

Create `config/samples/dev-cluster.yaml`:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: dev-cluster
  namespace: default
spec:
  version: "2.4.4"
  replicas: 3
  tls:
    enabled: true
    mode: OperatorManaged
  storage:
    size: "10Gi"
```

```sh
kubectl apply -f config/samples/dev-cluster.yaml
kubectl get pods -l openbao.org/cluster=dev-cluster -w
```

## Documentation

  * [User Guide](docs/user-guide.md): Configuration, Backups, Upgrades, and Day 2 Ops.
  * [Security & RBAC](docs/security.md): Threat model, RBAC design, and hardening guide.
  * [Architecture](docs/architecture.md): Internal controller design and state machines.
  * [Contributing](docs/contributing.md): Build instructions and testing strategy.

## Contributing

We welcome issues and pull requests. When contributing:

- Follow the coding guidelines in `AGENTS.md`.
- Ensure `go test ./...` and `golangci-lint` pass locally.
- Keep documentation in sync with any non-trivial behavior changes.

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
