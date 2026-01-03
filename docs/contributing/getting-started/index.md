# Getting Started with Contributing

Welcome! We are excited that you are interested in contributing to the OpenBao Operator. This section will guide you through setting up your environment and making your first contribution.

## Quick Start

<div class="grid cards" markdown>

- :material-source-fork: **1. Fork & Clone**

    ---

    Fork the repository on GitHub and clone it locally to start working.

    ```bash
    git clone https://github.com/YOUR-USERNAME/openbao-operator.git
    cd openbao-operator
    ```

- :material-laptop: **2. Setup Environment**

    ---

    Install the necessary tools (Go, Docker, Kind) to run the operator locally.

    [:material-arrow-right: Setup Guide](development.md)

- :material-source-branch: **3. Create Branch**

    ---

    Create a new feature branch from `main` for your changes.

    ```bash
    git checkour -b feature/my-awesome-feature
    ```

- :material-test-tube: **4. Test & Submit**

    ---

    Run the test suite and open a Pull Request.

    [:material-arrow-right: Testing Guide](../testing.md)

</div>

## Prerequisites

Ensure you have the following tools installed before starting:

<div class="grid cards" markdown>

- :simple-go: **Go 1.25+**

    ---

    The core language for the operator.

- :simple-docker: **Docker**

    ---

    Required for building container images.

- :simple-kubernetes: **Kind**

    ---

    (Kubernetes in Docker) for running local E2E tests.

- :simple-kubernetes: **kubectl**

    ---

    CLI for interacting with the cluster.

</div>

## Ways to Contribute

We welcome many types of contributions beyond just code:

| Type | Description |
| :--- | :--- |
| **Bug Fixes** | Fix issues found in existing functionality. Check for the `bug` label. |
| **Features** | Add new capabilities to the operator. Discuss large changes in an Issue first. |
| **Documentation** | Improve guides, fix typos, or add examples to help other users. |
| **Tests** | Improve test coverage or add new E2E scenarios. |
| **Refactoring** | Improve code quality and maintainability without changing behavior. |

!!! tip "First Time?"
    Look for issues labeled with `good first issue` on our [GitHub Issues](https://github.com/dc-tec/openbao-operator/issues) page.

## Next Steps

<div class="grid cards" markdown>

- **Development Setup**

    ---
    Step-by-step guide to configuring your local dev environment.

    [:material-arrow-right: Go to Setup](development.md)

- **Coding Standards**

    ---
    Learn about our code style, linting rules, and conventions.

    [:material-arrow-right: View Standards](../standards/index.md)

</div>
