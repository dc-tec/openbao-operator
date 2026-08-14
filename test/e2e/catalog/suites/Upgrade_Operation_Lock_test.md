# Upgrade Strategies: Operation Lock Contention

Source: `test/e2e/Upgrade_Operation_Lock_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `upgrade-backup-lock-contention` | holds a manual backup request until the rolling upgrade lock is released | active | `operation-lock`, `rolling-upgrade`, `backup-queueing` | `upgrade`, `backup`, `operation-lock`, `slow`, `e2e-anchor` |
| `backup-first-bluegreen-operation-order` | processes a completed backup before starting a blue-green upgrade | active | `operation-lock`, `backup-owner-first`, `bluegreen-upgrade` | `upgrade`, `backup`, `operation-lock`, `slow`, `e2e-anchor` |
| `backup-first-rolling-operation-order` | processes a completed backup before starting a rolling upgrade | active | `operation-lock`, `backup-owner-first`, `rolling-upgrade` | `upgrade`, `backup`, `operation-lock`, `slow`, `e2e-anchor` |

## `upgrade-backup-lock-contention`

Path: `Upgrade Strategies: Operation Lock Contention > holds a manual backup request until the rolling upgrade lock is released`

State: `active`

Generated fallback ID: `upgrade-operation-lock-holds-a-manual-backup-request-until-805368c4`

Covers: `operation-lock`, `rolling-upgrade`, `backup-queueing`

Labels: `upgrade`, `backup`, `operation-lock`, `slow`, `e2e-anchor`

Recorded checkpoints:
- starting a rolling upgrade
- waiting for the upgrade controller to hold the cluster operation lock
- requesting a manual backup while the upgrade lock is held
- verifying the backup request remains queued behind the active upgrade
- waiting for the upgrade to complete
- re-triggering reconcile so the queued backup request is picked up immediately
- verifying the queued backup starts once the upgrade lock is released


## `backup-first-bluegreen-operation-order`

Path: `Upgrade Strategies: Operation Lock Contention > processes a completed backup before starting a blue-green upgrade`

State: `active`

Generated fallback ID: `upgrade-operation-lock-processes-a-completed-backup-before-starting-45917e47`

Covers: `operation-lock`, `backup-owner-first`, `bluegreen-upgrade`

Labels: `upgrade`, `backup`, `operation-lock`, `slow`, `e2e-anchor`


## `backup-first-rolling-operation-order`

Path: `Upgrade Strategies: Operation Lock Contention > processes a completed backup before starting a rolling upgrade`

State: `active`

Generated fallback ID: `upgrade-operation-lock-processes-a-completed-backup-before-starting-a10c668a`

Covers: `operation-lock`, `backup-owner-first`, `rolling-upgrade`

Labels: `upgrade`, `backup`, `operation-lock`, `slow`, `e2e-anchor`
