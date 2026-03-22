---
description: Coding standards for OpenBao Operator covering Go style, error handling, Kubernetes patterns, security practices, and project conventions.
---

# Coding Standards

These coding standards ensure consistency and quality across the OpenBao Operator codebase. All contributors—human or AI-assisted—must follow these guidelines.

> **Guiding Principle:** *"Clear is better than clever."* We prioritize readability, maintainability, and explicit error handling over terse or "magical" code.

## Standards Directory

<div class="grid cards" markdown>

- **Go Style**

    ---

    Formatting, naming conventions, and idiomatic Go usage.

    [Read Guide](go-style.md)

- **Error Handling**

    ---

    Proper error wrapping, checking, and defining well-known errors.

    [Read Guide](error-handling.md)

- **Generated Artifacts**

    ---

    Handling auto-generated code (CRDs, DeepCopy, RBAC).

    [Read Guide](generated-artifacts.md)

- **K8s Patterns**

    ---

    Operator best practices: idempotency, context, and status updates.

    [Read Guide](kubernetes-patterns.md)

- **Security Practices**

    ---

    Secure coding, input validation, and secrets handling.

    [Read Guide](security-practices.md)

- **Conventions**

    ---

    Project-specific rules for metrics, logging, and extensive testing.

    [Read Guide](project-conventions.md)

- **Conventional Commits**

    ---

    Standardized commit messages for automated changelogs.

    [Read Guide](conventional-commits.md)

</div>

## Quick Reference

### The Golden Rules

<Callout type="success" title="Must Do">

- [x] **Format Code:** Always run `gofmt` or `goimports`.
- [x] **Linting:** Pass `golangci-lint` with the default configuration.
- [x] **Respect Architecture Boundaries:** Keep controller imports and package layering aligned with `.ast-grep/policy/architecture-boundaries.yml`.
- [x] **Keep ast-grep Rule Families Green:** Ensure `architecture-boundary`, `runtime-safety`, `reconcile-shape`, `status-ownership`, and `rbac-vap` checks pass.
- [x] **Wrap Errors:** Use `fmt.Errorf("...: %w", err)` to preserve context.
- [x] **Structured Logs:** Use `log.Info("msg", "key", "value")` instead of `Printf`.
- [x] **Test Logic:** Write table-driven unit tests for all business logic.
- [x] **Verify:** Run the full check suite:
    `make bootstrap && make doctor && make ci-core`

</Callout>

<Callout type="failure" title="Must NOT Do">

- [ ] **No `interface{}`:** Avoid `any` types without rigorous justification.
- [ ] **No Secret Logs:** Never log keys, tokens, or passwords.
- [ ] **No Blockers:** Do NOT use `time.Sleep()` in reconcilers. Use `RequeueAfter`.
- [ ] **No RBAC Annotations:** Do NOT use `+kubebuilder:rbac` on the Cluster controller.
- [ ] **No Shelling Out:** Do NOT exec out to `kubectl` or CLI tools; use Go libraries.

</Callout>

## See Also

- [Testing Guide](../testing.md) — Detailed test requirements
- [Documentation Style Guide](../docs-style-guide.md) — Writing docs

