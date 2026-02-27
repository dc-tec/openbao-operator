# Continuous Integration

We use GitHub Actions for all CI checks. The pipeline is designed to be deterministic and reproducible locally.

## 1. CI Pipeline

The pipeline runs on every PR and `main` push.

```mermaid
graph TD
    PR([PR Created]) --> Static
    PR --> Build
    PR --> Unit

    subgraph Static [Static Analysis]
        Lint[Lint & Tidy]
        Gen[Verify Generated]
        Helm[Verify Helm]
        Sec[Security Scan]
    end

    subgraph Build [Build Artifacts]
        Docs[Build Docs]
        Chart[Lint Chart]
    end

    subgraph Unit [Verification]
        Sanity[Unit Tests]
        Compat[OpenBao Compat]
    end

    Static --> E2E{E2E Tests}
    Build --> E2E
    Unit --> E2E

    E2E --> Smoke["Smoke Tests"]
    E2E --> Full["Full Matrix (Nightly)"]

    classDef process fill:transparent,stroke:#9333ea,stroke-width:2px,color:#fff;
    classDef read fill:transparent,stroke:#60a5fa,stroke-width:2px,color:#fff;
    classDef write fill:transparent,stroke:#22c55e,stroke-width:2px,color:#fff;
    
    class PR process;
    class Static,Build,Unit,E2E process;
    class Lint,Gen,Helm,Sec,Docs,Chart,Sanity,Compat,Smoke,Full read;
```

## 2. CI vs Local Commands ("The Rosetta Stone")

Run these locally to debug CI failures.

| CI Job | Local Command | Description |
| :--- | :--- | :--- |
| **Lint Check** | `make lint` | Runs `golangci-lint` |
| **Formatting** | `make verify-fmt` | Checks `gofmt` compliance |
| **Dependencies** | `make verify-tidy` | Ensures `go.mod` is clean |
| **Generators** | `make verify-generated` | Checks for drift in CRDs/RBAC |
| **Helm Sync** | `make verify-helm` | Checks drift in `charts/` (including values/schema validation) |
| **Unit Tests** | `make test-ci` | Runs unit + integration tests |
| **Compatibility** | `make verify-openbao-config-compat` | Checks HCL against upstream OpenBao |
| **Architecture Report (local)** | `make report-internal-deps` | Generates internal package dependency report and graphs (report-only warnings) |

## 2.1 Pull Request Standards Gates

We enforce PR metadata and commit standards in CI (`.github/workflows/pr-standards.yml`):

- PR title must follow Conventional Commits.
- PR description must include required sections from `.github/pull_request_template.md`.
- Commit-subject checks are informational only (not enforced).

Because this repository uses squash merge, the PR title gate is the effective release-facing commit message gate for `main`.

## 3. End-to-End Testing

We use [Kind](https://kind.sigs.k8s.io/) for E2E tests.

CI optimization model:

- Build once, reuse many: CI builds fast E2E images once per workflow and reuses pinned digest refs across shards.
- Trusted PR requirement: the optimized E2E lane runs for same-repository PRs and branch pushes (it pushes temporary images to GHCR).
- Hybrid routing:
  - Path-driven by default (`changes` job decides whether E2E is needed, and whether backup/upgrade/hardened lanes are relevant).
  - PR labels can expand scope:
    - `backup` includes backup/restore slow lane.
    - `upgrades` includes upgrade/chaos slow lane.
    - `ci:full-e2e` forces the full suite (except OpenShift lane).
- Hardened PR lane focuses on hardened/GitOps coverage (ACME-focused checks are left to main/nightly/release).

### Prerequisites

- [x] Docker running
- [x] `kubectl` installed
- [x] 4 CPU / 8GB RAM recommended

### Running Tests

=== ":material-rocket-launch: Smoke Test (Fast)"
    Runs a subset of critical tests. Best for quick feedback.

    ```sh
    make test-e2e-ci \
      KIND_NODE_IMAGE=kindest/node:v1.34.3 \
      E2E_LABEL_FILTER=smoke \
      E2E_PARALLEL_NODES=1
    ```

=== ":material-flask: Full Suite (Thorough)"
    Runs the entire test matrix (Upgrade, Backup, Restore, etc).

    ```sh
    make test-e2e-ci KIND_NODE_IMAGE=kindest/node:v1.34.3
    ```

=== ":material-bug: Debug Mode"
    Keeps the cluster alive after failure for inspection.

    ```sh
    make test-e2e-ci E2E_SKIP_CLEANUP=true
    ```

## 4. Security Checks

We run vulnerability scanning on every PR.

=== "Govulncheck"
    Detects known vulnerabilities in Go dependencies.

    ```sh
    go install golang.org/x/vuln/cmd/govulncheck@latest
    govulncheck -test ./...
    ```

=== "Trivy"
    Scans the operator image for OS vulnerabilities.

    ```sh
    make security-scan IMG=ghcr.io/dc-tec/openbao-operator:latest
    ```

    !!! note "Expected RBAC findings (skipped in Trivy FS)"
        Trivy's Kubernetes misconfiguration rules flag several **intentionally privileged** RBAC manifests/templates
        (e.g. tenant template roles, single-tenant mode, and provisioner cleanup permissions).
        We skip these specific files in CI and in `make security-scan` using Trivy's `--skip-files` flags.

        If you modify RBAC under `config/rbac/`, `dist/install.yaml`, or the chart RBAC templates, and Trivy starts failing,
        update the skip list in:

        - `.github/workflows/ci.yml` (Trivy FS step)
        - `.github/workflows/nightly.yml` (Trivy FS step)
        - `Makefile` (`security-scan` target)

## 5. Documentation Build

Docs are built with MkDocs and Material.

```sh
# Local preview
make docs-serve

# Build distribution (checks internal links)
make docs-build
```

!!! tip "Preview Deployment"
    CI currently validates docs with `make docs-build` but does not publish a per-PR preview URL.
    Use `make docs-serve` locally for interactive preview.

## 6. Performance Baseline Capture (GitHub Runners)

Use the dedicated workflow to capture baseline evidence on the same runner class used by CI.

1. Open **Actions** -> **Performance Baseline Capture**.
2. Run the workflow on `main` (or the target release branch) with default inputs unless you intentionally want a different run count/timeout.
3. Download the uploaded artifact containing:
   - `hack/perf/baseline/kind-v1.34.3-baseline.json`
   - `hack/perf/thresholds/kind-v1.34.3.yaml`
4. Commit those files in a normal PR and use release/weekly perf workflows to validate thresholds (`verify-perf`).

## 7. Weekly Performance Regression Gate

The `Performance Regression Weekly` workflow runs full `make verify-perf` weekly and can also be run manually.

- Scheduled failures automatically open (or update) an issue titled `Weekly performance regression detected`.
- Release workflow still enforces full `verify-perf` as a hard gate.
