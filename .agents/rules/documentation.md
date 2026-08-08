---
trigger: always_on
glob: "**/*.md"
description: Documentation standards for the OpenBao Operator
---

# Documentation Standards

See [Documentation Style Guide](../../website/EDITORIAL.md).

## Structure

- `website/content/docs/` — Planned stable documentation
- `website/content-versions/next/docs/` — Unreleased `main` documentation
- `website/content-versions/<minor>/docs/` — Frozen or maintained stable minor lines
- `website/content/contribute/` — Contributor guides shared across documentation lines

## Rules

1. Update docs when behavior changes
2. Use the Hugo `callout` shortcode for warnings and notes
3. Use relative links between docs
4. Tables must have consistent spacing (MD060)
5. Code blocks must specify language
6. Keep lines under 120 characters when possible

## Key Locations to Update

Non-trivial changes should update:

- `website/content/docs/architecture/` for new components
- `website/content/docs/` for user-facing behavior
- `website/content/docs/security/` for security-related changes
- the matching `content-versions` line when a change applies to stable or `next`

## Building Docs

```sh
make docs-serve         # Local preview (CI-equivalent)
make docs-build         # Validate (CI-equivalent; strict)

# Or use the pinned local Nix invocation documented in website/README.md.
```
