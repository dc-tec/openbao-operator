# Setting Up Your Environment

This guide covers everything you need to build, run, and test the OpenBao Operator locally.

## Prerequisites

Ensure you have the following installed before starting:

!!! tip "Required Tools"
    - [x] **Go** v1.26.1+ (`go version`)
    - [x] **Docker** v28.3.3+ (`docker version`)
    - [x] **kubectl** v1.33+ (`kubectl version --client`)
    - [x] **Helm** v3+ (`helm version`)
    - [x] **Trivy** (`trivy --version`)
    - [x] **Python** 3 with `venv` support (`python3 -m venv --help`)
    - [x] **Node.js** 20+ with `npm` (`npm --version`)
    - [x] **Kubernetes Cluster** v1.33+ (Kind, Minikube, or Cloud)

!!! tip "Optional but Recommended"
    - [x] **Kind** for running local E2E tests (`kind version`)
    - [x] **k9s** for inspecting the cluster state

## Recommended First Run

Bootstrap the repo-managed toolchain once, then validate your workstation:

```sh
make bootstrap
make doctor
```

If you use Tilt for the local Kubernetes inner loop on your host machine, install it separately
from the [official Tilt docs](https://docs.tilt.dev/install.html). The provided devcontainer
preinstalls Tilt together with the other external prerequisites.

If you open the repository in the provided devcontainer, these external prerequisites are
preinstalled for you and the container runs `make bootstrap && make doctor` automatically
after creation.

## Development Workflow

We support two main development workflows. Choose the one that fits your current task.

=== ":material-laptop: Local Development (Fast Loop)"

    Best for rapid iteration on logic. The operator runs as a generic Go binary on your laptop and connects to the cluster via your kubeconfig.

    1.  **Clone the Repo:**
        ```sh
        git clone https://github.com/dc-tec/openbao-operator.git
        cd openbao-operator
        ```

    2.  **Bootstrap Tooling:**
        Install the repo-managed local tools and generated dependencies.
        ```sh
        make bootstrap
        make doctor
        ```

    3.  **Install CRDs:**
        Apply the Custom Resource Definitions to your target cluster.
        ```sh
        make install
        ```

    4.  **Run Operator:**
        Start the cluster controller locally. It will use your `~/.kube/config`.
        ```sh
        make run-controller
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

    2.  **Bootstrap Tooling:**
        ```sh
        make bootstrap
        make doctor
        ```

    3.  **Build & Load Image:**
        Build the docker image and load it directly into Kind (no registry needed).
        ```sh
        make docker-build IMG=openbao-operator:dev
        kind load docker-image openbao-operator:dev --name openbao-dev
        ```

    4.  **Deploy:**
        Install CRDs and deploy the operator manifests.
        ```sh
        make deploy IMG=openbao-operator:dev
        ```

    5.  **Verify:**
        ```sh
        kubectl get pods -n openbao-operator-system
        ```

=== ":material-rocket-launch-outline: Tilt (Fast Cluster Loop)"

    Best for repeated controller changes against a local cluster when you want automatic image rebuilds,
    deploys, logs, and a few manual check actions in one UI.

    1.  **Prepare a Local Cluster:**
        Tilt is intended for local kube contexts such as `kind-*`, `k3d-*`, `minikube`, or `docker-desktop`.

    2.  **Bootstrap Tooling:**
        ```sh
        make bootstrap
        make doctor
        ```

    3.  **Start Tilt:**
        ```sh
        make tilt-up
        ```

    4.  **Optional: Override Helper Image Defaults**
        By default, the Tilt workflow uses the locally built manager image and published `edge` helper images.
        Override these when needed:
        ```sh
        tilt up -- --operator-version=edge
        ```

    5.  **Stop Tilt:**
        ```sh
        make tilt-down
        ```

## Common Make Targets

Use `make help` to see all available commands, or refer to this cheatsheet:

| Category | Target | Description |
| :--- | :--- | :--- |
| **Setup** | `make bootstrap` | Install repo-managed tools, envtest assets, docs deps, and ast-grep. |
| | `make doctor` | Validate external prerequisites for the main contributor workflow. |
| | `make tilt-up` / `tilt-down` | Start or stop the local Kubernetes dev loop in Tilt. |
| **Build** | `make build` | Compile the binary to `bin/manager`. |
| | `make docker-build` | Build the container image. |
| **Run / Debug** | `make air-controller` | Run the cluster controller with live reload via Air. |
| | `make air-provisioner` | Run the provisioner with live reload via Air. |
| | `make debug-controller` | Launch the controller under Delve. |
| | `make debug-provisioner` | Launch the provisioner under Delve. |
| | `make debug-test PKG=./path TEST=TestName` | Debug a single Go test package with Delve. |
| **Deploy** | `make install` / `uninstall` | Install/Remove CRDs. |
| | `make deploy` / `undeploy` | Deploy/Remove Operator & RBAC. |
| **Verify** | `make lint-ci` | Run code linters and ast-grep guardrails. |
| | `make ci-core` | Run the PR-equivalent local gate (everything except E2E). |
| | `make generate-ast-rules` | Generate ast-grep architecture boundary rules from policy. |
| | `make verify-arch-policy` | Verify generated ast-grep architecture rules are up-to-date. |
| | `make test` | Run unit tests. |
| | `make test-sum` | Run unit tests with gotestsum output and JUnit XML under `dist/test/`. |
| | `make test-integration` | Run integration tests (envtest). |
| | `make test-integration-sum` | Run integration tests with gotestsum output and JUnit XML under `dist/test/`. |
| | `make report-internal-deps` | Generate local internal dependency graph/report artifacts. |
| | `make report-internal-deps-snapshot` | Generate report and save/update a local baseline snapshot. |
| | `make report-internal-deps-diff` | Compare baseline snapshot vs current report (delta output). |
| **Perf** | `make bench` | Run Go benchmarks (`go test -bench`) across the selected package set. |
| | `make bench-save` | Save benchmark output under `dist/bench/` for later comparison. |
| | `make bench-compare OLD=... NEW=...` | Compare benchmark runs with `benchstat`. |
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
    Validating Webhooks require the K8s API server to reach the operator. When running `make run-controller`, this is difficult. Use the **Cluster Deployment** method to test webhooks.

??? question "How do I confirm my workstation has everything required?"
    Run:
    ```sh
    make doctor
    ```
    The command reports missing external tools and explains which workflows they block.

??? question "What does the Tilt workflow build?"
    Tilt rebuilds the manager image locally and deploys the controller and provisioner from `config/default`.
    It also renders helper-image environment variables so the local operator defaults to published helper images
    that match `OPERATOR_VERSION` (default: `edge`).
