# Contributing to OpenBao Operator

Welcome! We are glad you want to contribute to the OpenBao Operator. Whether you're fixing a bug, adding a feature, or improving documentation, this guide will help you get started.

## Quick Start

<div class="grid cards" markdown>

- :material-flag-checkered: **Getting Started**

    ---

    New to the project? Start here to set up your environment and make your first contribution.

    [:material-arrow-right: Start Here](getting-started/index.md)

- :material-ruler-square: **Coding Standards**

    ---

    Review our conventions for Go code, Kubernetes patterns, and error handling.

    [:material-arrow-right: View Standards](standards/index.md)

- :material-test-tube: **Testing Guide**

    ---

    Learn how to run our unit tests, integration tests, and E2E suites.

    [:material-arrow-right: Run Tests](testing.md)

</div>

## AI-Assisted Contributions

!!! info "Policy on AI Tools"
    We welcome contributions that leverage AI tools! However, you **must** verify all generated code.

    *   **Responsibility:** You are responsible for the code you submit. "The AI wrote it" is not a valid excuse for bugs or security issues.
    *   **Context:** Use our `.agent/rules/` to ensure your AI assistant follows our project conventions.
    *   **Quality:** PRs that appear to be low-effort AI dumps ("slop PRs") without proper understanding or testing will be closed.

## Submitting Changes

Follow this checklist to ensure your Pull Request is ready for review:

1. **Code:** Conform to the [Coding Standards](standards/index.md).
2. **Tests:** Add unit tests for logic and E2E tests for features.
3. **Validation:** Run the local verification suite:

    ```bash
    # 1. Linting & Formatting
    make lint-config lint verify-fmt verify-tidy

    # 2. Generated Artifacts
    make verify-generated verify-helm

    # 3. Tests
    make test-ci
    ```

4. **Commits:** Write clear, descriptive commit messages.
5. **Documentation:** Update relevant docs if behavior changed.

## Documentation Guides

<div class="grid cards" markdown>

- **CI/CD Pipeline**

    ---
    Understanding our GitHub Actions workflows.

    [:material-arrow-right: CI Guide](ci.md)

- **Release Management**

    ---
    How we version and release the operator.

    [:material-arrow-right: Release Process](release-management.md)

- **SDLC Overview**

    ---
    Our development lifecycle and philosophy.

    [:material-arrow-right: Read SDLC](sdlc.md)

- **Docs Style Guide**

    ---
    Writing documentation for the project.

    [:material-arrow-right: Style Guide](docs-style-guide.md)

</div>

## Quick Links

- [Go Style](standards/go-style.md) — Formatting, naming, idioms
- [Error Handling](standards/error-handling.md) — Error patterns
- [Kubernetes Patterns](standards/kubernetes-patterns.md) — Operator best practices
- [Security Practices](standards/security-practices.md) — Secure coding
- [Project Conventions](standards/project-conventions.md) — Metrics, logging, testing
