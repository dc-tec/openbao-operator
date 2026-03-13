---
description: Known limitations and explicit non-goals for OpenBao Operator 0.x release lines.
---

# Known Limitations & Non-Goals

This page lists current limitations and explicit non-goals for the `0.x` series.

## 1. Pre-GA API Stability

!!! warning "Pre-1.0 behavior"
    The Operator is currently pre-GA (`v1alpha1`). Minor releases may include breaking changes.

See [Deprecation Policy](deprecation-policy.md) for lifecycle details.

## 2. CRD Version Surface

- Current served/storage CRD API is `openbao.org/v1alpha1`.
- Multi-version conversion webhooks are not part of the current pre-GA scope.

## 3. Operator Upgrade / Downgrade Constraints

- Operator upgrades are supported; see [Operator Upgrade Compatibility](operator-upgrade-compatibility.md).
- Operator downgrades are not treated as a normal rollback mechanism.

## 4. External Backup Object Deletion

`OpenBaoCluster.spec.deletionPolicy: DeleteAll` currently removes PVC-backed data but does **not** delete external object storage backups (S3/GCS/Azure).

See [Deletion Policy](../user-guide/openbaocluster/operations/deletion.md).

## 5. etcd Encryption Verification

The operator cannot directly verify cluster-level etcd encryption-at-rest settings and surfaces a warning condition instead (`EtcdEncryptionWarning`).

## 6. Helm CRD Lifecycle Semantics

- Helm does not automatically upgrade CRDs.
- Helm does not delete CRDs on uninstall.

Use release `crds.yaml` assets for CRD lifecycle operations.

## 7. Channel Usage Constraints

- `edge` and `nightly` channels are for validation, not production support.
- Production should pin explicit stable versions.

## 8. Support Window

Support is focused on the latest stable release line.

See [Support & Maintenance Policy](support-policy.md).
