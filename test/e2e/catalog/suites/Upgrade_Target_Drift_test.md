# Upgrade Strategies: Blue/Green Drift

Source: `test/e2e/Upgrade_Target_Drift_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `bluegreen-target-drift-restart` | abandons an outdated green revision and converges on the new desired image | active | `bluegreen-drift`, `target-revision-drift`, `stale-green-cleanup` | `upgrade`, `bluegreen`, `slow` |

## `bluegreen-target-drift-restart`

Path: `Upgrade Strategies: Blue/Green Drift > abandons an outdated green revision and converges on the new desired image`

State: `active`

Generated fallback ID: `upgrade-target-drift-abandons-an-outdated-green-revision-and-72046d97`

Covers: `bluegreen-drift`, `target-revision-drift`, `stale-green-cleanup`

Labels: `upgrade`, `bluegreen`, `slow`

Recorded checkpoints:
- starting a blue/green upgrade
- waiting for the first green revision to enter an early phase
- changing the desired target image while the first green revision is still in flight
- verifying the stale green workload is cleaned up
- verifying the upgrade restarts from the new desired revision and completes
- reconfirming the steady-state workload stays singular after the restart


