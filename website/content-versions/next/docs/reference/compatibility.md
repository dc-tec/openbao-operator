---
title: Compatibility
description: Validated Kubernetes and OpenBao versions, CI coverage, and the production upgrade stance.
eyebrow: Reference
weight: 1
verifiedBy:
  - test/e2e/suites.yaml
  - test/e2e/e2e_versions.go
  - test/e2e/Upgrade_Strategies_test.go
  - test/e2e/backup_restore_test.go
  - internal/service/upgrade/bluegreen/version_compatibility.go
---

The operator requires Kubernetes 1.33 or newer. The rows below describe the current validation baseline, not a blanket guarantee for every cloud, distribution, or topology.

## Kubernetes versions

| Version | Validation | Support posture |
| --- | --- | --- |
| 1.36.x | Pull request, nightly, release, Helm, and upgrade coverage | Primary validated line |
| 1.35.x | Daily smoke, alternating weekly full coverage, and release coverage | Validated compatibility line |
| 1.34.x | Daily smoke, alternating weekly full coverage, and release coverage | Validated compatibility line |
| 1.33.x | Not validated for the current release line | Upstream end of life; validate in staging before adoption |
| OpenShift | Manifest, Helm, admission, and focused platform coverage | Validate on the target cluster |

## OpenBao versions

| Version | Validation | Production note |
| --- | --- | --- |
| 2.6.3 | Default CI, nightly, and release target; local lifecycle, rolling-upgrade, backup, and restore qualification | Unreleased `main` baseline; validate the exact environment in staging |
| 2.6.2 | OpenBao Operator 0.5.0 release target; rolling-upgrade source for local 2.6.3 qualification | Use the latest qualified security patch |
| Other 2.6.x | Not individually release-gated | Validate the exact patch in staging |
| 2.5.x | Config compatibility and rolling-upgrade source coverage | Validate the transition in staging |
| 2.4.x | Config compatibility | Upgrade before a new production rollout |
| 2.3.x | Not validated | Out of support scope |

{{< callout type="warning" title="OpenBao 2.6 BlueGreen limitation" >}}
OpenBao 2.6.0 changed its internal request-forwarding gRPC service name.
[OpenBao 2.6.3](https://github.com/openbao/openbao/releases/tag/v2.6.3) fixes cross-version request forwarding,
but the operator still blocks pre-2.6 to 2.6-or-newer `BlueGreen` transitions until an exact version pair is qualified.
The forwarding fix does not establish datastore downgrade compatibility.
{{< /callout >}}

## OpenBao 2.6.3 qualification

Local qualification uses Kubernetes 1.34.3 on Linux ARM64 with static unseal and operator-managed TLS. It covers
fresh self-initialization, file audit output, a 2.6.2 to 2.6.3 rolling upgrade with three voters and one read replica,
and S3 backup and restore with RustFS. The restore checks voter replacement, read-replica recovery, and retained data.
These checks do not qualify external unseal providers, cloud storage credentials, or a pre-2.6 `BlueGreen` transition.

## Production upgrade rule

Validate the exact Kubernetes distribution, OpenBao version, unseal mechanism, storage, networking, and upgrade strategy in staging before changing production—even when both versions appear in the validated matrix.
