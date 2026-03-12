---
description: Dependency license policy for OpenBao Operator, including allowed licenses, MPL-2.0 obligations, forbidden licenses, and CI enforcement.
---

# Dependency License Policy

Use this page to review the dependency license policy enforced for shipped OpenBao Operator binaries.

!!! note
    This page defines project policy for maintainers and contributors. It is not legal advice.

## Scope

The blocking license gate covers these shipped Go binaries:

- `./cmd/controller`
- `./cmd/bao-backup`
- `./cmd/bao-upgrade`
- `./cmd/provisioner`

!!! tip
    Run license verification in vendored mode (`-mod=vendor`) so the result matches the build and release path.

## Allowed Licenses

These licenses are allowed without additional license-specific review:

- `Apache-2.0`
- `BSD-2-Clause`
- `BSD-3-Clause`
- `ISC`
- `MIT`
- `Unicode-DFS-2016`

These licenses generally require preserving notices rather than reciprocal source disclosure across the larger work.

## Allowed With Obligations

The following license is allowed, but it requires explicit maintainer care:

- `MPL-2.0`

### Why `MPL-2.0` Is Allowed

`MPL-2.0` is file-level copyleft, not whole-program copyleft.
It can be distributed as part of a larger Apache-2.0 project without converting the whole operator to `MPL-2.0`.

### Required Handling for `MPL-2.0`

When you add or update a dependency under `MPL-2.0`:

1. Do not patch vendored `MPL-2.0` files casually.
2. Do not copy `MPL-2.0` code into first-party files without explicit review.
3. Preserve upstream license and notice files during redistribution.
4. If modified `MPL-2.0` files are redistributed in source or binary form, keep corresponding source for those modified files available as required by the license.
5. Call out any newly introduced `MPL-2.0` dependency in the PR description so the review trail is explicit.

## Forbidden Licenses

Do not ship dependencies under these licenses or license classes:

- `GPL-2.0`
- `GPL-3.0`
- `AGPL-3.0`
- `LGPL-2.1`
- `LGPL-3.0`
- `SSPL`
- `BUSL` / `BSL`
- `Elastic License`
- `Commons-Clause`
- Non-commercial, no-derivatives, field-of-use-restricted, or source-available-only licenses
- `Unknown`, `NOASSERTION`, and custom or unrecognized licenses

These licenses introduce obligations or restrictions that do not fit the operator's distribution model.

## Enforcement

The repository enforces license policy in two layers:

1. GitHub dependency review checks newly introduced dependencies on pull requests against the allowlist in `.github/dependency-review-config.yml`.
2. `go-licenses` performs a full shipped-dependency graph check in vendored mode through `make license-check`.

The `go-licenses` check is the canonical full-tree gate for this repository.

## Local Verification

Run these commands after changing dependencies:

```sh
make verify-vendor
make license-check
make license-report
```

`make license-report` writes:

- `dist/licenses/go-licenses-report.csv`
- `dist/licenses/go-licenses-report.stderr.log`

## Updating the Policy

Treat allowlist changes as maintainer-level changes.

When you change the allowed or forbidden license set:

1. A short explanation in the PR description.
2. A matching update to this document.
3. A matching update to the machine-enforced CI configuration.
