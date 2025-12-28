# Development Environment

## 1. Prerequisites

- **Go:** version v1.25.5+ (check with `go version`)
- **Docker:** version 28.3.3+ (check with `docker version`)
- **kubectl:** version v1.33+ (check with `kubectl version --client`)
- **Access to a Kubernetes cluster:** v1.33+ (for testing; see `docs/compatibility.md`)
- **Kind:** For local E2E testing (optional but recommended)
- **Make:** For running build targets

You should also have:

- Permissions to install CRDs and cluster-scoped RBAC
- A default `StorageClass` for StatefulSet PVCs

## 2. Clone the Repository

```sh
git clone https://github.com/openbao/openbao-operator.git
cd openbao-operator
```

## 3. Build & Run

### 3.1 Local Development

**Build the operator binary:**

```sh
make build
```

This compiles the operator binary to `bin/manager`.

**Run the operator locally:**

```sh
make run
```

This runs the operator locally using `kubectl` to connect to your configured Kubernetes cluster. The operator will use your local kubeconfig (typically `~/.kube/config`).

**Note:** Running locally requires:

- A Kubernetes cluster accessible via your kubeconfig
- CRDs installed (see below)
- Proper RBAC permissions

### 3.2 Install CRDs

Before running the operator, install the Custom Resource Definitions:

```sh
make install
```

This installs the `OpenBaoCluster` CRD into your cluster.

### 3.3 Deploy the Operator

**Build and push a container image:**

```sh
make docker-build docker-push IMG=<your-registry>/openbao-operator:tag
```

**Deploy to the cluster:**

```sh
make deploy IMG=<your-registry>/openbao-operator:tag
```

**NOTE:** If you encounter RBAC errors, you may need to grant yourself cluster-admin privileges or be logged in as admin.

### 3.4 Uninstall

**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs (CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

### 3.5 Build Targets

Run `make help` to see all available make targets:

```sh
make help
```

Common targets include:

- `make build` - Build the operator binary
- `make run` - Run the operator locally
- `make install` - Install CRDs
- `make uninstall` - Uninstall CRDs
- `make deploy` - Deploy the operator to the cluster
- `make docker-build` - Build the container image
- `make docker-push` - Push the container image
- `make manifests` - Generate manifests (CRDs, RBAC, etc.)
- `make generate` - Generate code (deepcopy, clientset, etc.)
- `make test` - Run unit tests
- `make test-integration` - Run integration tests
- `make lint` - Run linters
