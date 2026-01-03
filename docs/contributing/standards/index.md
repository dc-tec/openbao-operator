# Coding Standards

These coding standards ensure consistency and quality across the OpenBao Operator codebase. All contributors—human or AI-assisted—must follow these guidelines.

> **Guiding Principle:** *"Clear is better than clever."* We prioritize readability, maintainability, and explicit error handling over terse or "magical" code.

## Standards Directory

<div class="grid cards" markdown>

- :simple-go: **Go Style**

    ---

    Formatting, naming conventions, and idiomatic Go usage.

    [:material-arrow-right: Read Guide](go-style.md)

- :material-alert-circle-outline: **Error Handling**

    ---

    Proper error wrapping, checking, and defining sentinel errors.

    [:material-arrow-right: Read Guide](error-handling.md)

- :material-cog-refresh: **Generated Artifacts**

    ---

    Handling auto-generated code (CRDs, DeepCopy, RBAC).

    [:material-arrow-right: Read Guide](generated-artifacts.md)

- :simple-kubernetes: **K8s Patterns**

    ---

    Operator best practices: idempotency, context, and status updates.

    [:material-arrow-right: Read Guide](kubernetes-patterns.md)

- :material-shield-lock: **Security Practices**

    ---

    Secure coding, input validation, and secrets handling.

    [:material-arrow-right: Read Guide](security-practices.md)

- :material-bookmark-box-multiple: **Conventions**

    ---

    Project-specific rules for metrics, logging, and extensive testing.

    [:material-arrow-right: Read Guide](project-conventions.md)

</div>

## Quick Reference

### The Golden Rules

!!! success "Must Do"
    - [x] **Format Code:** Always run `gofmt` or `goimports`.
    - [x] **Linting:** Pass `golangci-lint` with the default configuration.
    - [x] **Wrap Errors:** Use `fmt.Errorf("...: %w", err)` to preserve context.
    - [x] **Structured Logs:** Use `log.Info("msg", "key", "value")` instead of `Printf`.
    - [x] **Test Logic:** Write table-driven unit tests for all business logic.
    - [x] **Verify:** Run the full check suite:
        `make lint verify-fmt verify-tidy verify-generated verify-helm test-ci`

!!! failure "Must NOT Do"
    - [ ] **No `interface{}`:** Avoid `any` types without rigorous justification.
    - [ ] **No Secret Logs:** Never log keys, tokens, or passwords.
    - [ ] **No Blockers:** Do NOT use `time.Sleep()` in reconcilers. Use `RequeueAfter`.
    - [ ] **No RBAC Annotations:** Do NOT use `+kubebuilder:rbac` on the Cluster controller.
    - [ ] **No Shelling Out:** Do NOT exec out to `kubectl` or CLI tools; use Go libraries.

## See Also

- [Testing Guide](../testing.md) — Detailed test requirements
- [Documentation Style Guide](../docs-style-guide.md) — Writing docs
