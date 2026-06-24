# Hardened profile (External TLS + Transit auto-unseal + SelfInit)

Source: `test/e2e/Cluster_Profile_Hardened_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `hardened-bluegreen-upgrade` | performs a hardened blue/green upgrade | active | _none_ | `profile-hardened`, `security`, `cluster`, `upgrade`, `bluegreen`, `hardened` |
| `hardened-rolling-upgrade` | performs a hardened rolling upgrade | active | _none_ | `profile-hardened`, `security`, `cluster`, `upgrade`, `rolling`, `hardened` |
| `hardened-self-init-auto-unseal` | creates a Hardened cluster that self-initializes and stays unsealed across restarts | active | _none_ | `profile-hardened`, `security`, `cluster` |
| `cluster-profile-hardened-provisions-tenant-rbac-via-openbaotenant-90cecec2` | provisions tenant RBAC via OpenBaoTenant | active | _none_ | `profile-hardened`, `security`, `cluster` |
| `cluster-profile-hardened-verifies-raft-autopilot-is-configured-with-f66c15e2` | verifies Raft Autopilot is configured with cleanup_dead_servers enabled | active | _none_ | `profile-hardened`, `security`, `cluster` |

## `hardened-bluegreen-upgrade`

Path: `Hardened profile (External TLS + Transit auto-unseal + SelfInit) > Hardened Blue/Green Upgrade > performs a hardened blue/green upgrade`

State: `active`

Generated fallback ID: `cluster-profile-hardened-performs-a-hardened-blue-green-upgrade-6ad6aecb`

Covers: _none_

Labels: `profile-hardened`, `security`, `cluster`, `upgrade`, `bluegreen`, `hardened`

Recorded checkpoints:
- verifying the tenant is provisioned for the hardened blue/green cluster
- ensuring the transit credentials secret exists for hardened blue/green unseal
- creating external TLS secrets for the hardened blue/green cluster
- creating a hardened cluster configured for blue/green upgrades
- writing a secret before the hardened blue/green upgrade
- triggering the hardened blue/green upgrade
- verifying the hardened blue/green upgrade starts
- waiting for the hardened blue/green upgrade to complete
- verifying the test secret persists after the hardened blue/green upgrade


## `hardened-rolling-upgrade`

Path: `Hardened profile (External TLS + Transit auto-unseal + SelfInit) > Hardened Rolling Upgrade > performs a hardened rolling upgrade`

State: `active`

Generated fallback ID: `cluster-profile-hardened-performs-a-hardened-rolling-upgrade-514bbfcf`

Covers: _none_

Labels: `profile-hardened`, `security`, `cluster`, `upgrade`, `rolling`, `hardened`

Recorded checkpoints:
- verifying the tenant is provisioned for the hardened upgrade cluster
- ensuring the transit credentials secret exists for hardened cluster unseal
- creating external TLS secrets for the hardened upgrade cluster
- creating a hardened cluster configured for rolling upgrades
- writing a secret before the hardened upgrade
- triggering the hardened rolling upgrade
- verifying the hardened rolling upgrade starts
- waiting for the hardened rolling upgrade to complete
- verifying the test secret persists after the hardened upgrade


## `hardened-self-init-auto-unseal`

Path: `Hardened profile (External TLS + Transit auto-unseal + SelfInit) > creates a Hardened cluster that self-initializes and stays unsealed across restarts`

State: `active`

Generated fallback ID: `cluster-profile-hardened-creates-a-hardened-cluster-that-self-a3cf527d`

Covers: _none_

Labels: `profile-hardened`, `security`, `cluster`

Recorded checkpoints:
- creating external TLS secrets required for TLS mode External
- verifying transit token secret can be read from file and access infra-bao transit key
- waiting for OpenBaoCluster to be observed by the API server
- verifying NetworkPolicy was created
- checking for prerequisite resources (ConfigMap and TLS Secrets)
- waiting for StatefulSet to be created
- waiting for the StatefulSet pod to become Ready (proves auto-unseal worked)
- waiting for status.initialized=true (self-init, no operator init)
- triggering reconcile to ensure status is updated
- verifying the encrypted recovery-key backup exists for the declared recipients
- verifying the documented hardened production readiness condition
- asserting root token and static unseal secrets do NOT exist
- deleting pods to verify auto-unseal works after restart (maintenance mode)


## `cluster-profile-hardened-provisions-tenant-rbac-via-openbaotenant-90cecec2`

Path: `Hardened profile (External TLS + Transit auto-unseal + SelfInit) > provisions tenant RBAC via OpenBaoTenant`

State: `active`

Covers: _none_

Labels: `profile-hardened`, `security`, `cluster`

Recorded checkpoints:
- verifying OpenBaoTenant is provisioned


## `cluster-profile-hardened-verifies-raft-autopilot-is-configured-with-f66c15e2`

Path: `Hardened profile (External TLS + Transit auto-unseal + SelfInit) > verifies Raft Autopilot is configured with cleanup_dead_servers enabled`

State: `active`

Covers: _none_

Labels: `profile-hardened`, `security`, `cluster`

Recorded checkpoints:
- ensuring public service exists before creating verification pod
- ensuring public service has ready endpoints
- allowing verification pod egress to OpenBao cluster (NetworkPolicy)
- reading autopilot configuration via JWT authenticated request
