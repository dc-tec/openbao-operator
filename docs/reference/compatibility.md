# Compatibility Matrix: OpenBao Operator

This document defines the supported Kubernetes and OpenBao versions for this
Operator. It is intended to be kept up to date for every tagged release and
referenced by testing and upgrade documentation.

## 1. Kubernetes Versions

The table below lists the Kubernetes minor versions that are explicitly
validated in CI for this operator. The Operator is intended to support
**Kubernetes v1.33 and newer**; older versions are not supported.

| Kubernetes Version | Status     | Notes                                |
|--------------------|------------|--------------------------------------|
| v1.33              | Supported  | Primary CI coverage                  |
| v1.34              | Supported  | Primary CI coverage                  |
| v1.35              | Supported  | Primary CI coverage                  |

## 2. OpenBao Versions

The table below lists the OpenBao versions that have been tested with this
operator release.

| OpenBao Version | Status     | Notes                           |
|-----------------|-----------|---------------------------------|
| 2.4.x           | Supported | Primary target for this release |

CI expectations for this range:

- PR CI validates operator-generated HCL against upstream OpenBao config parsing for **min supported + latest supported** (currently `2.4.0` and `2.4.4`).
- Nightly CI validates the same check across a broader 2.4.x patch set.

> **Note:** Platform teams should validate new Kubernetes or OpenBao versions
> in a non-production environment before rolling them out widely, even if they
> fall within a “Supported” range.

## 3. Testing Expectations

- CI SHOULD run at least:
  - Unit/integration tests on one Kubernetes version from the table above.
  - E2E tests (kind + OpenBao) on at least the primary Kubernetes version and
    OpenBao 2.4.x.
- When adding support for a new Kubernetes minor:
  - Add it to this matrix.
  - Ensure E2E tests pass on that version.
- When deprecating a Kubernetes minor:
  - Move its status to “Deprecated” and update user-facing docs accordingly.
