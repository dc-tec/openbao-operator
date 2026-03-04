# Ast-Grep Architecture Policy

This directory is the source of truth for architecture boundary guardrails that are enforced by ast-grep.

- Policy file: `architecture-boundaries.yml`
- Generator: `hack/tools/ast_rulegen`
- Generated rules: `.ast-grep/rules/generated/architecture-boundary/*.yml`

The policy currently encodes:

- `layerCoverage`: required classification for top-level `internal/*` packages
- `controllerCoverage`: required explicit policy entries for controller packages
- `serviceImportRoots` and `adapterImportRoots`: global import domains for controller approval rules
- `controllerBoundaries`: per-controller app facade and approved import allowlists
- `globalImportBoundaries`: repository-wide import guardrails that are generated into ast-grep rules
  - `disallowImports`: module-local import roots (resolved under `modulePath/`)
  - `disallowExternalImports`: external import roots (matches root and subpackages)
  - `disallowExternalExactImports`: exact external import paths only

## Why policy is here (not under `hack/`)

`hack/` is used for tooling implementation.  
`.ast-grep/policy/` is used for guardrail intent and ownership, next to the rules and tests it drives.

This keeps policy updates and ast-grep validation changes in one place while still allowing generator code to live in `hack/tools/`.

## Workflow

1. Edit `.ast-grep/policy/architecture-boundaries.yml`.
2. Run `make generate-ast-rules`.
3. Run `make verify-arch-policy`.
4. Run `make test-ast lint-ast`.

When adding a new `internal/*` top-level package or a new controller package:

- update `layerCoverage`
- update `controllerBoundaries` (or `controllerCoverage.exempt`, if intentionally exempt)
