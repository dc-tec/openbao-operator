# Contributing

Thank you for your interest in contributing to the OpenBao Operator!

## Getting Started

Full documentation lives under `contribute/`. Start here:

- [Contributing Overview](contribute/index.mdx) — Submitting changes, AI notice
- [Getting Started](contribute/getting-started/index.md) — First-time setup
- [Development Setup](contribute/getting-started/development.md) — Build & run locally
- [Coding Standards](contribute/standards/index.md) — Code style & patterns
- [Testing](contribute/testing.md) — Unit, integration, E2E tests
- [CI/CD](contribute/ci.md) — Pipeline overview

## AI-Assisted Contributions

We welcome AI-assisted contributions. However, all code must meet our quality standards:

- **Understand what you submit** — You are responsible for your code
- **Follow standards** — See [Coding Standards](contribute/standards/index.md)
- **Test your changes** — PRs must pass CI

> **Tip**: Configure your AI tool to use `.agents/rules/` for project-specific rules.

## Local Checks (PR-equivalent)

```sh
make bootstrap
make doctor
make ci-core
```
