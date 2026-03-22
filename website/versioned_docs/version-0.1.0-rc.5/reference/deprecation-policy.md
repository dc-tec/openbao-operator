---
description: Deprecation and API lifecycle policy for OpenBao Operator, including pre-1.0 compatibility expectations and migration requirements.
---

# Deprecation & API Lifecycle Policy

This document defines how the OpenBao Operator evolves APIs and behavior across releases.

## 1. Scope

This policy applies to:

- CRD API versions (`openbao.org/*`)
- CRD fields (`spec`, `status`)
- User-visible defaults and behavior contracts
- Operator installation and upgrade workflows

## 2. Stability Level (Pre-1.0)

<Callout type="warning" title="Pre-1.0 contract">

Until `1.0.0`, the Operator is in pre-GA (`v1alpha1`) and may introduce breaking changes.

</Callout>

For `0.x` releases:

- **Minor releases (`0.Y.0`)** may contain breaking API/behavior changes.
- **Patch releases (`0.Y.Z`)** should avoid intentional breaking changes.
- Security, safety, or data-integrity fixes may require urgent behavior changes.

## 3. Deprecation Process

When we deprecate a field or behavior, we aim to do all of the following in the same release:

1. Mark deprecation in API comments (source of truth for generated API docs).
2. Document deprecation in [API Reference](api.md) and release notes/changelog.
3. Provide a migration path and an example replacement.

## 4. Removal Policy

For pre-1.0 (`0.x`) releases:

- Removals are expected in **minor** releases, not patch releases.
- We aim to keep deprecated fields available for at least one minor release when feasible.
- In exceptional cases (security/safety), removal may happen earlier with explicit release notes.

## 5. Migration Requirements

Any breaking or removing change must include:

- A migration section in release notes/changelog.
- Clear "before/after" manifests for affected CRDs.
- Upgrade sequencing notes if operator upgrade order matters.

## 6. Kubernetes API Versioning Mechanics

The project currently serves a single CRD API version (`v1alpha1`).

When introducing additional CRD versions (for example `v1beta1` or `v1`), we will use Kubernetes-native version lifecycle controls:

- `served` / `storage` version flags
- `deprecated: true`
- `deprecationWarning`

## 7. Operator User Guidance

Before upgrading:

1. Read release notes for deprecations and migrations.
2. Apply CRD updates first when required.
3. Validate changes in staging before production rollout.

Related references:

- [Compatibility Matrix](compatibility.md)
- [Operator Upgrade Compatibility](operator-upgrade-compatibility.md)
- [Release Management](/contribute/release-management)
