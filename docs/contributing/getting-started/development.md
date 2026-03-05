# Setting Up Your Environment

This guide covers everything you need to build, run, and test the OpenBao Operator locally.

## Prerequisites

Ensure you have the following installed before starting:

!!! tip "Required Tools"
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
| **Verify** | `make lint-ci` | Run code linters and ast-grep guardrails. |
| | `make generate-ast-rules` | Generate ast-grep architecture boundary rules from policy. |
| | `make verify-arch-policy` | Verify generated ast-grep architecture rules are up-to-date. |
| | `make test` | Run unit tests. |
| | `make test-integration` | Run integration tests (envtest). |
| | `make report-internal-deps` | Generate local internal dependency graph/report artifacts. |
| | `make report-internal-deps-snapshot` | Generate report and save/update a local baseline snapshot. |
| | `make report-internal-deps-diff` | Compare baseline snapshot vs current report (delta output). |
| **Generate**| `make manifests` | Regenerate CRD YAMLs and RBAC. |
| | `make generate` | Regenerate `deepcopy` code. |

## Internal Dependency Report

Use the local architecture report tooling to inspect internal package dependencies.

```sh
make report-internal-deps
```

Artifacts are written to `dist/architecture/`:

- `internal-dependency-report.md`
- `internal-dependency-edges.tsv`
- `internal-dependency-graph.dot`
- `internal-dependency-graph.mmd`
- `adapter-boundary-audit.md`

The command is report-only and does not fail on policy findings.

Intentional policy exceptions are tracked in:

- `hack/architecture/dependency-policy-exceptions.tsv`

The adapter boundary audit classifies runtime adapter-to-adapter edges as:

- `exception`: edge is explicitly tracked in `hack/architecture/dependency-policy-exceptions.tsv`.
- `untracked`: edge is not tracked and should be reviewed before merge.
To track trends locally across refactors, save a baseline snapshot and compare against it:

```sh
make report-internal-deps-snapshot
make report-internal-deps
make report-internal-deps-diff
```

You can override the compared files when needed:

```sh
BASELINE_REPORT=/path/to/old-report.md CURRENT_REPORT=/path/to/new-report.md make report-internal-deps-diff
```

## Ast-Grep Architecture Policy

Ast-grep architecture boundary rules are generated from:

- `.ast-grep/policy/architecture-boundaries.yml`

When adding new top-level `internal/*` packages or new controller packages, update this policy first so coverage checks stay complete.

Regenerate and verify before pushing:

```sh
make generate-ast-rules
make verify-arch-policy
make test-ast
make lint-ast
```

## Troubleshooting

??? question "RBAC Errors during `make deploy`?"
    Ensure your current kubeconfig user has `cluster-admin` privileges.
    ```sh
    kubectl create clusterrolebinding my-admin --clusterrole=cluster-admin --user=$(gcloud config get-value account)
    ```

??? question "Webhooks failing locally?"
    Validating Webhooks require the K8s API server to reach the operator. When running `make run`, this is difficult. Use the **Cluster Deployment** method to test webhooks.
