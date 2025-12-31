# Contributing to the OpenBao Operator

Welcome! We're glad you want to contribute to the OpenBao Operator.

## Getting Started

New to the project? Start here:

1. [Getting Started](getting-started/index.md) — Fork, clone, and set up your environment
2. [Development Setup](getting-started/development.md) — Build and run locally
3. [Coding Standards](standards/index.md) — Code style and patterns

## AI-Assisted Contributions

This project was developed with AI assistance, and we welcome contributions that leverage AI tools.
However, all contributions—AI-assisted or not—must meet our quality standards:

- **Understand what you're submitting.** You are responsible for the code you contribute.
- **Follow the coding standards** in our [Coding Standards](standards/index.md) guide.
- **Test your changes.** PRs must pass CI and include appropriate test coverage.
- **Write meaningful commit messages** that explain the "why," not just the "what."

PRs that appear to be low-effort AI-generated content ("slop PRs") without proper review,
testing, or understanding will be closed without merge.

!!! tip "For AI Tool Users"
    Configure your AI assistant to use `.agent/rules/` as workspace rules. These rules
    reference the project's coding standards and ensure AI-generated code follows
    our conventions.

## Submitting Changes

1. **Fork the repository** and create a feature branch
2. **Make your changes** following the [Coding Standards](standards/index.md)
3. **Add tests** for new functionality
4. **Run tests locally**:

   ```sh
   make test
   make lint
   ```

5. **Update documentation** if needed
6. **Commit with clear messages** explaining the change
7. **Submit a pull request**

## Documentation

| Guide | Description |
|-------|-------------|
| [Getting Started](getting-started/index.md) | First-time contributor setup |
| [Coding Standards](standards/index.md) | Code style and patterns |
| [Testing](testing.md) | Unit, integration, and E2E tests |
| [CI/CD](ci.md) | Continuous integration pipeline |
| [Release Management](release-management.md) | Versioning and releases |
| [SDLC Overview](sdlc.md) | Development lifecycle |
| [Documentation Style Guide](docs-style-guide.md) | Writing documentation |

## Quick Links

- [Go Style](standards/go-style.md) — Formatting, naming, idioms
- [Error Handling](standards/error-handling.md) — Error patterns
- [Kubernetes Patterns](standards/kubernetes-patterns.md) — Operator best practices
- [Security Practices](standards/security-practices.md) — Secure coding
- [Project Conventions](standards/project-conventions.md) — Metrics, logging, testing

## Additional Resources

- [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)
- [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)
- [Effective Go](https://go.dev/doc/effective_go)
