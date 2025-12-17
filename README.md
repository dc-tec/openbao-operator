# OpenBao Supervisor Operator

The OpenBao Supervisor Operator manages the full lifecycle of OpenBao clusters on Kubernetes. It adopts a **Supervisor Pattern**, delegating data consistency to OpenBao (Raft) while managing the external ecosystem (PKI, Upgrades, Backups).

## Architecture Overview

The Operator is split into two distinct components to enforce strict security boundaries:

1.  **Provisioner:** A privileged component that creates namespaces and grants specific, limited RBAC permissions.
2.  **Controller:** A low-privilege workload that manages OpenBao clusters. It cannot access resources outside of its provisioned namespaces.

```mermaid
flowchart LR
  %% High-level view showing the separation of Provisioner and Controller
  user["Platform team / tenant admin"]

  api["Kubernetes API server"]
  
  subgraph "Operator System"
    provisioner["Provisioner<br/>(Cluster Privileges)"]
    controller["Controller<br/>(Namespace Privileges)"]
  end

  tenantNS["Tenant namespace<br/>- OpenBaoCluster (CR)<br/>- Tenant RBAC"]
  clusterNS["OpenBao cluster namespace<br/>(managed resources)"]

  access["OpenBao endpoint<br/>(Service)"]
  obj["Object storage<br/>(S3 / GCS / Azure)"]

  user -->|apply CRs| api
  
  provisioner <-->|watch Tenants| api
  provisioner -->|1. create NS & RBAC| tenantNS
  
  controller <-->|watch Clusters| api
  controller -->|2. reconcile resources| clusterNS

  clusterNS --> access
  user -->|OpenBao API| access

  controller -->|stream snapshots| obj

  classDef core fill:#EBFBEE,stroke:#2F9E44,color:#1B5E20;
  classDef api fill:#F1F3F5,stroke:#495057,color:#212529;
  classDef ns fill:#FFF4E6,stroke:#F08C00,color:#7A3E00;

  class provisioner,controller core;
  class api api;
  class tenantNS,clusterNS ns;
```

```mermaid
flowchart TB
  %% Per OpenBaoCluster: showing the Sentinel interaction
  clusterCtl["Reconciler"]
  
  subgraph "Data Plane"
    secrets["Secrets"]
    config["ConfigMap"]
    
    subgraph "StatefulSet Pod"
      bao["OpenBao Container"]
      sentinel["Sentinel Sidecar"]
    end
    
    svc["Service"]
  end

  clusterCtl -->|reconcile| secrets
  clusterCtl -->|reconcile| config
  clusterCtl -->|reconcile| svc
  
  config -.->|mount| bao
  secrets -.->|mount| bao
  
  sentinel -.->|watch for drift| config
  sentinel -.->|watch for drift| secrets
  sentinel -->|trigger fast-path| clusterCtl

  classDef controller fill:#EBFBEE,stroke:#2F9E44,color:#1B5E20;
  classDef workload fill:#FFF4E6,stroke:#F08C00,color:#7A3E00;
  classDef sidecar fill:#E7F5FF,stroke:#1C7ED6,color:#1864AB;
  
  class clusterCtl controller;
  class secrets,config,bao,svc workload;
  class sentinel sidecar;
```

## Key Features

  * **Zero Trust Architecture:** The Controller runs with minimal permissions. Access is granted dynamically per-tenant by the Provisioner.
  * **Automated PKI:** Manages internal Root CA, rotates certificates, and triggers hot-reloads.
  * **Raft-Aware Upgrades:** Orchestrates safe, step-down based rolling upgrades.
  * **Disaster Recovery:** Streams snapshots directly to S3/GCS/Azure without disk buffering.
  * **Secure by Default:** Rootless, read-only filesystem, strict NetworkPolicies, and auto-unseal.
  * **Multi-Tenant:** Namespace-scoped isolation with strict RBAC boundaries.

## Quick Start

**Prerequisites**: Kubernetes v1.33+ (see `docs/compatibility.md`), `kubectl`, `helm` (optional).

### 1. Install the Operator

```sh
make install
make deploy IMG=ghcr.io/openbao/openbao-operator:latest
```

### 2. Deploy a Cluster

If you are running in multi-tenant mode, provision your target namespace first (recommended):

```sh
kubectl create namespace default || true
kubectl apply -f - <<'YAML'
apiVersion: openbao.org/v1alpha1
kind: OpenBaoTenant
metadata:
  name: default-tenant
  namespace: openbao-operator-system
spec:
  targetNamespace: default
YAML
```

Create a minimal `OpenBaoCluster`:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: dev-cluster
  namespace: default
spec:
  version: "2.4.4"
  image: "openbao/openbao:2.4.4"
  replicas: 3
  tls:
    enabled: true
    mode: OperatorManaged
    rotationPeriod: "720h"
  storage:
    size: "10Gi"
```

```sh
kubectl apply -f config/samples/dev-cluster.yaml
kubectl get pods -l openbao.org/cluster=dev-cluster -w
```

## Documentation

  * [Docs Index](docs/README.md)
  * [User Guide](docs/user-guide/README.md): Configuration, Backups, Upgrades, and Day 2 Ops.
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
