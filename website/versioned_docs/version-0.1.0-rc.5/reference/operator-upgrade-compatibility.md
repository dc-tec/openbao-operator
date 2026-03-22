---
description: Operator upgrade compatibility policy, supported upgrade paths, rollback stance, and required CRD upgrade sequence.
---

# Operator Upgrade Compatibility

This document defines supported upgrade paths for the OpenBao Operator itself.

## 1. Supported Upgrade Paths

Supported paths:

- Stable patch upgrades (`0.Y.Z -> 0.Y.(Z+1)`)
- Stable minor upgrades (`0.Y.Z -> 0.(Y+1).0`) with release note review

Recommended path:

- Upgrade sequentially across minors (do not skip multiple minors at once).

Not supported:

- Operator downgrades as a routine rollback strategy.
- Treating Edge/Nightly as production upgrade baselines.

## 2. Required CRD Upgrade Order

<Callout type="warning" title="CRD-first upgrade">

Apply CRDs before upgrading the Helm release when CRDs changed.

</Callout>

Use the release assets:

```bash
kubectl apply -f https://github.com/dc-tec/openbao-operator/releases/download/X.Y.Z/crds.yaml
helm upgrade openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
  --namespace <operator-namespace>
```

This matches the operator installation guidance in [Installation](../user-guide/operator/installation.md#upgrading).

## 3. Upgrade Safety Checklist

Before upgrade:

- Confirm target version in [Compatibility Matrix](compatibility.md).
- Take and verify backups for managed clusters.
- Review release notes for deprecations and migrations.

After upgrade:

- Verify operator Deployments are `Running`.
- Verify CRD version and controller readiness.
- Verify managed cluster conditions and recent events.

## 4. Rollback Strategy

If an upgrade introduces issues:

1. Prefer forward-fix on a newer stable release.
2. If rollback is required, treat it as a recovery operation (staging validation first).
3. Use backup/restore runbooks for data-path recovery scenarios.

Related references:

- [Backups](../user-guide/openbaocluster/operations/backups.md)
- [Restore](../user-guide/openbaorestore/restore.md)
- [Recovery Runbooks](../user-guide/openbaocluster/recovery/no-leader.md)

## 5. API Compatibility During Operator Upgrades

- Current CRD API is `openbao.org/v1alpha1`.
- Pre-1.0 API evolution rules are documented in [Deprecation Policy](deprecation-policy.md).
- Always validate manifests against the generated [API Reference](api.md) before rollout.

