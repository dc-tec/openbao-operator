# Setting Up Your Environment

This guide covers everything you need to build, run, and test the OpenBao Operator locally.

## Prerequisites

Ensure you have the following installed before starting:

!!! check "Required Tools"
    - [x] **Go** v1.25.5+ (`go version`)
    - [x] **Docker** v28.3.3+ (`docker version`)
    - [x] **kubectl** v1.33+ (`kubectl version --client`)
    - [x] **Kubernetes Cluster** v1.33+ (Kind, Minikube, or Cloud)

!!! tip "Optional but Recommended"
    - [x] **Kind** for running local E2E tests (`kind version`)
    - [x] **k9s** for inspecting the cluster state
    - [x] **golangci-lint** for local linting

## Development Workflow

We support two main development workflows. Choose the one that fits your current task.

=== ":material-laptop: Local Development (Fast Loop)"

    Best for rapid iteration on logic. The operator runs as a generic Go binary on your laptop and connects to the cluster via your kubeconfig.

    1.  **Clone the Repo:**
        ```sh
        git clone https://github.com/dc-tec/openbao-operator.git
        cd openbao-operator
        ```

    2.  **Install CRDs:**
        Apply the Custom Resource Definitions to your target cluster.
        ```sh
        make install
        ```

    3.  **Run Operator:**
        Start the controller locally. It will use your `~/.kube/config`.
        ```sh
        make run
        ```

    !!! warning "Limitations"
        - Webhooks may not work locally without tunneling (ngrok).
        - NetworkPolicies cannot be tested this way.

=== ":simple-kubernetes: Cluster Deployment (Integration Loop)"

    Best for testing full lifecycle, webhooks, and RBAC permissions. The operator runs as a Pod inside the cluster.

    1.  **Start Kind Cluster:**
        ```sh
        kind create cluster --name openbao-dev
        ```

    2.  **Build & Load Image:**
        Build the docker image and load it directly into Kind (no registry needed).
        ```sh
        make docker-build IMG=openbao-operator:dev
        kind load docker-image openbao-operator:dev --name openbao-dev
        ```

    3.  **Deploy:**
        Install CRDs and deploy the operator manifests.
        ```sh
        make deploy IMG=openbao-operator:dev
        ```

    4.  **Verify:**
        ```sh
        kubectl get pods -n openbao-operator-system
        ```

## Common Make Targets

Use `make help` to see all available commands, or refer to this cheatsheet:

| Category | Target | Description |
| :--- | :--- | :--- |
| **Build** | `make build` | Compile the binary to `bin/manager`. |
| | `make docker-build` | Build the container image. |
| **Deploy** | `make install` / `uninstall` | Install/Remove CRDs. |
| | `make deploy` / `undeploy` | Deploy/Remove Operator & RBAC. |
| **Verify** | `make lint` | Run code linters. |
| | `make test` | Run unit tests. |
| | `make test-integration` | Run integration tests (envtest). |
| **Generate**| `make manifests` | Regenerate CRD YAMLs and RBAC. |
| | `make generate` | Regenerate `deepcopy` code. |

## Troubleshooting

??? question "RBAC Errors during `make deploy`?"
    Ensure your current kubeconfig user has `cluster-admin` privileges.
    ```sh
    kubectl create clusterrolebinding my-admin --clusterrole=cluster-admin --user=$(gcloud config get-value account)
    ```

??? question "Webhooks failing locally?"
    Validating Webhooks require the K8s API server to reach the operator. When running `make run`, this is difficult. Use the **Cluster Deployment** method to test webhooks.
