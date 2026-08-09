# Contributing

Thank you for your interest in contributing to the OpenBao Operator!

## Getting Started

Full documentation lives under `website/content/contribute/`. Start here:

- [Contributing Overview](website/content/contribute/_index.md) — Submitting changes and contributor routes
- [Getting Started](website/content/contribute/setup.md) — First-time and development setup
- [Coding Standards](website/content/contribute/standards.md) — Code style and patterns
- [Testing](website/content/contribute/testing.md) — Unit, integration, and E2E tests
- [CI/CD](website/content/contribute/ci.md) — Pipeline overview

## AI-Assisted Contributions

We welcome AI-assisted contributions. However, all code must meet our quality standards:

- **Understand what you submit** — You are responsible for your code
- **Follow standards** — See [Coding Standards](website/content/contribute/standards.md)
- **Test your changes** — PRs must pass CI

> **Tip**: Configure your AI tool to use `.agents/rules/` for project-specific rules.

## Local Checks (PR-equivalent)

```sh
devenv test
devenv tasks run operator:bootstrap
devenv tasks run operator:doctor
devenv tasks run operator:ci-core
```
