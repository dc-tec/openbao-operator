---
trigger: always_on
glob: "**/*.go"
description: Go coding standards for the OpenBao Operator
---

# Go Coding Standards

Follow the project coding standards in `docs/contributing/standards/`.

## References

- [Go Style](docs/contributing/standards/go-style.md)
- [Error Handling](docs/contributing/standards/error-handling.md)
- [Kubernetes Patterns](docs/contributing/standards/kubernetes-patterns.md)
- [Security Practices](docs/contributing/standards/security-practices.md)
- [Project Conventions](docs/contributing/standards/project-conventions.md)

## Critical Rules

1. Format with `gofmt`/`goimports`
2. Pass `golangci-lint` with project config
3. Wrap errors with context: `fmt.Errorf("context: %w", err)`
4. Use structured logging with fields: `cluster_namespace`, `cluster_name`
5. Do NOT add `+kubebuilder:rbac` to OpenBaoCluster controller
6. Do NOT shell out to kubectl, helm, or cloud CLIs
7. Do NOT use `interface{}` or `any` without justification
8. Do NOT log secrets, tokens, or keys
9. Do NOT spawn goroutines in reconcilers
10. Do NOT use `time.Sleep()` — use controller-runtime rate limiting
