# Ast-Grep Architecture Policy

This directory is the source of truth for architecture boundary guardrails that are enforced by ast-grep.

- Policy file: `architecture-boundaries.yml`
- Generator: `hack/tools/ast_rulegen`
- Generated rules: `.ast-grep/rules/generated/architecture-boundary/*.yml`

The policy currently encodes:

- `layerCoverage`: required classification for runtime package roots under `internal/`
  - top-level entries such as `controller`, `app`, and `port`
  - grouped entries such as `service/upgrade`, `adapter/config`, and `platform/constants`
- `controllerCoverage`: required explicit policy entries for controller packages
- `serviceImportRoots` and `adapterImportRoots`: global import domains for controller approval rules
- `controllerBoundaries`: per-controller app facade and approved import allowlists
- `serviceBoundaries`: per-service approved service import allowlists
- `appBoundaries`: app-package approved service import allowlists
- `globalImportBoundaries`: repository-wide import guardrails that are generated into ast-grep rules
  - `disallowImports`: module-local import roots (resolved under `modulePath/`)
  - `disallowExternalImports`: external import roots (matches root and subpackages)
  - `disallowExternalExactImports`: exact external import paths only

## Workflow

1. Edit `.ast-grep/policy/architecture-boundaries.yml`.
2. Run `make generate-ast-rules`.
3. Run `make verify-arch-policy`.
4. Run `make test-ast lint-ast`.

When adding a new top-level runtime package, grouped layer package, or controller package:

- update `layerCoverage`
- update `controllerBoundaries` (or `controllerCoverage.exempt`, if intentionally exempt)
