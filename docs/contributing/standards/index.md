# Coding Standards

These coding standards ensure consistency and quality across the OpenBao Operator codebase. All contributors—human or AI-assisted—must follow these guidelines.

**Guiding Principle:** *"Clear is better than clever."* We prioritize readability, maintainability, and explicit error handling over terse or "magical" code.

## Standards

| Standard | Description |
|----------|-------------|
| [Go Style](go-style.md) | Formatting, naming, idioms |
| [Error Handling](error-handling.md) | Error wrapping, checking, sentinel errors |
| [Kubernetes Patterns](kubernetes-patterns.md) | Idempotency, context, logging, RBAC |
| [Security Practices](security-practices.md) | File permissions, crypto, input validation |
| [Project Conventions](project-conventions.md) | Metrics, logging, CRD evolution, testing |

## Quick Reference

### Must Do

- ✅ Format with `gofmt` or `goimports`
- ✅ Pass `golangci-lint` with default config
- ✅ Wrap errors with context using `%w`
- ✅ Use structured logging with standard fields
- ✅ Write table-driven tests

### Must NOT Do

- ❌ Use `interface{}` or `any` without justification
- ❌ Log secrets, keys, or tokens
- ❌ Use `time.Sleep()` in controllers
- ❌ Add `+kubebuilder:rbac` annotations to OpenBaoCluster controller
- ❌ Shell out to external tools

## See Also

- [Testing](../testing.md) — Test patterns and requirements
- [Documentation Style Guide](../docs-style-guide.md) — Writing documentation
