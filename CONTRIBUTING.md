# Contributing

Thank you for your interest in contributing to the OpenBao Operator!

## Getting Started

Full documentation lives under `docs/contributing/`. Start here:

- [Contributing Overview](docs/contributing/index.md) — Submitting changes, AI notice
- [Getting Started](docs/contributing/getting-started/index.md) — First-time setup
- [Development Setup](docs/contributing/getting-started/development.md) — Build & run locally
- [Coding Standards](docs/contributing/standards/index.md) — Code style & patterns
- [Testing](docs/contributing/testing.md) — Unit, integration, E2E tests
- [CI/CD](docs/contributing/ci.md) — Pipeline overview

## AI-Assisted Contributions

We welcome AI-assisted contributions. However, all code must meet our quality standards:

- **Understand what you submit** — You are responsible for your code
- **Follow standards** — See [Coding Standards](docs/contributing/standards/index.md)
- **Test your changes** — PRs must pass CI

> **Tip**: Configure your AI tool to use `.agent/rules/` for project-specific rules.

## Local Checks (PR-equivalent)

```sh
make lint-config lint
make verify-fmt
make verify-tidy
make verify-generated
make verify-helm
make test-ci
```
