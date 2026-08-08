---
trigger: always_on
glob: "**/*.go"
description: Go coding standards for the OpenBao Operator
---

# Go Coding Standards

- Follow the project coding standards in `website/content/contribute/standards.md`.
- Use the [coding-guidelines skill](../skills/coding-guidelines/SKILL.md)

## References

- [Project standards](../../website/content/contribute/standards.md)

## Critical Rules

1. Format with `gofmt`/`goimports`
2. Run `make lint-ci` for CI-equivalent linting (`golangci-lint` + ast-grep checks)
3. Keep ast-grep rule families green: `architecture-boundary`, `runtime-safety`, `reconcile-shape`, `status-ownership`, `rbac-vap`
4. Wrap errors with context: `fmt.Errorf("context: %w", err)`
5. Use structured logging with fields: `cluster_namespace`, `cluster_name`
6. In controllers, avoid legacy log keys (`namespace`, `name`) for request-scoped logs
7. Keep functions small (lint enforces cyclomatic complexity)
8. Keep Go lines under 120 chars when practical (lint enforces this in some dirs)
9. Do NOT add `+kubebuilder:rbac` to OpenBaoCluster controller
10. Do NOT shell out to kubectl, helm, or cloud CLIs
11. Avoid `interface{}` / `any` in controller and app core logic; use explicit bridge points when interop requires dynamic types
12. Do NOT log secrets, tokens, or keys
13. Do NOT spawn goroutines in reconcilers
14. Do NOT use `time.Sleep()` — use controller-runtime rate limiting
