---
trigger: always_on
glob: "**/*.md"
description: Documentation standards for the OpenBao Operator
---

# Documentation Standards

See [Documentation Style Guide](docs/contributing/docs-style-guide.md).

## Structure

- `docs/user-guide/` — User-facing guides
- `docs/architecture/` — Technical deep-dives
- `docs/security/` — Security documentation
- `docs/contributing/` — Contributor guides
- `docs/reference/` — API and compatibility references

## Rules

1. Update docs when behavior changes
2. Use admonitions for warnings/tips (`!!! warning`, `!!! tip`)
3. Use relative links between docs
4. Tables must have consistent spacing (MD060)
5. Code blocks must specify language
6. Keep lines under 120 characters when possible

## Key Locations to Update

Non-trivial changes should update:

- `docs/architecture/` for new components
- `docs/user-guide/` for user-facing changes
- `docs/security/` for security-related changes

## Building Docs

```sh
mkdocs serve    # Local preview
mkdocs build --strict  # Validate (fails on warnings)
```
