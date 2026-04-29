# Upgrade Strategies

Source: `test/e2e/Upgrade_Strategies_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `upgrade-strategies-holds-in-syncing-until-manual-promotion-f3915b17` | holds in Syncing until manual promotion after the pre-promotion hook succeeds | active | _none_ | `upgrade`, `upgrades`, `cluster`, `slow`, `bluegreen`, `verification` |
| `upgrade-strategies-executes-blue-green-upgrade-cycle-with-bc1db3b7` | executes Blue/Green upgrade cycle with pre-upgrade snapshot | active | _none_ | `upgrade`, `upgrades`, `cluster`, `slow`, `bluegreen`, `e2e-anchor` |
| `upgrade-strategies-aborts-before-promotion-when-the-pre-230729f6` | aborts before promotion when the pre-promotion hook fails | active | _none_ | `upgrade`, `upgrades`, `cluster`, `slow`, `bluegreen`, `verification`, `failure` |
| `upgrade-strategies-induces-executor-failure-and-validates-retry-f587a124` | induces executor failure and validates retry plus auto-abort behavior | active | _none_ | `upgrade`, `upgrades`, `cluster`, `slow`, `failure`, `bluegreen`, `e2e-anchor` |
| `upgrade-strategies-creates-a-tlsroute-and-reports-healthy-79be45e7` | creates a TLSRoute and reports healthy passthrough integration | active | _none_ | `upgrade`, `upgrades`, `cluster`, `slow`, `gateway`, `requires-gateway-api`, `tls-passthrough` |
| `upgrade-strategies-keeps-httproute-stable-and-switches-external-7a9e6dd5` | keeps HTTPRoute stable and switches external Service selector at cutover | active | _none_ | `upgrade`, `upgrades`, `cluster`, `slow`, `gateway`, `requires-gateway-api`, `bluegreen`, `e2e-anchor` |
| `upgrade-strategies-triggers-late-phase-rollback-after-promotion-c44db935` | triggers late-phase rollback after promotion failures and recovers when auth is restored | active | _none_ | `upgrade`, `upgrades`, `cluster`, `slow`, `failure`, `bluegreen`, `rollback` |
| `upgrade-strategies-recovers-a-failed-rolling-upgrade-after-5a3c4b87` | recovers a failed rolling upgrade after a retry request clears stale state | active | _none_ | `upgrade`, `upgrades`, `cluster`, `slow`, `rolling`, `recovery` |
| `upgrade-strategies-retries-a-failed-rolling-pre-upgrade-fade7ad8` | retries a failed rolling pre-upgrade snapshot before starting rollout | active | _none_ | `upgrade`, `upgrades`, `cluster`, `slow`, `rolling`, `snapshot`, `recovery` |
| `upgrade-strategies-performs-rolling-upgrade-2302a23d` | performs rolling upgrade | active | _none_ | `upgrade`, `upgrades`, `cluster`, `slow`, `rolling`, `e2e-anchor`, `read-replicas`, `read-replicas-rolling` |
| `upgrade-strategies-acknowledges-break-glass-and-resumes-rollback-f99ce2fa` | acknowledges break glass and resumes rollback after the upgrade policy is repaired | active | _none_ | `upgrade`, `upgrades`, `cluster`, `slow`, `chaos`, `bluegreen` |
| `upgrade-strategies-enters-safe-mode-when-rollback-consensus-e82ce327` | enters safe mode when rollback consensus repair job fails | active | _none_ | `upgrade`, `upgrades`, `cluster`, `slow`, `chaos`, `bluegreen` |

## `upgrade-strategies-holds-in-syncing-until-manual-promotion-f3915b17`

Path: `Upgrade Strategies > Blue/Green Syncing Gates > holds in Syncing until manual promotion after the pre-promotion hook succeeds`

State: `active`

Covers: _none_

Labels: `upgrade`, `upgrades`, `cluster`, `slow`, `bluegreen`, `verification`

Recorded checkpoints:
- Triggering a blue/green upgrade with manual promotion
- Waiting for Syncing hold with the hook completed successfully
- Consistently holding in Syncing while no promote request is set
- Approving promotion via spec.upgrade.requests.promote
- Verifying the upgrade resumes and completes cleanly


## `upgrade-strategies-executes-blue-green-upgrade-cycle-with-bc1db3b7`

Path: `Upgrade Strategies > Blue/Green Upgrade > executes Blue/Green upgrade cycle with pre-upgrade snapshot`

State: `active`

Covers: _none_

Labels: `upgrade`, `upgrades`, `cluster`, `slow`, `bluegreen`, `e2e-anchor`

Recorded checkpoints:
- Writing a secret before upgrade
- Ensuring the external Service exists for zero-downtime probing
- Continuously probing Service availability during the full Blue/Green upgrade
- Triggering upgrade
- Verifying pre-upgrade snapshot job is created
- Waiting for pre-upgrade snapshot job to complete
- Waiting for upgrade to complete
- Verifying upgrade completed successfully with snapshots
- Verifying Service availability remained continuous during the upgrade
- Verifying critical blue/green executor actions completed successfully
- Verifying secret persists after upgrade


## `upgrade-strategies-aborts-before-promotion-when-the-pre-230729f6`

Path: `Upgrade Strategies > Blue/Green Validation Hook Failure > aborts before promotion when the pre-promotion hook fails`

State: `active`

Covers: _none_

Labels: `upgrade`, `upgrades`, `cluster`, `slow`, `bluegreen`, `verification`, `failure`

Recorded checkpoints:
- Writing a secret before the failed validation hook upgrade
- Triggering a blue/green upgrade guarded by a failing pre-promotion hook
- Waiting for the pre-promotion hook to fail after green finishes syncing
- Verifying the upgrade aborts safely before promotion
- Verifying promotion never starts after the validation hook fails
- Verifying the original cluster remains readable after the failed upgrade


## `upgrade-strategies-induces-executor-failure-and-validates-retry-f587a124`

Path: `Upgrade Strategies > Failure Scenarios > induces executor failure and validates retry plus auto-abort behavior`

State: `active`

Covers: _none_

Labels: `upgrade`, `upgrades`, `cluster`, `slow`, `failure`, `bluegreen`, `e2e-anchor`

Recorded checkpoints:
- Triggering a Blue/Green upgrade
- Waiting for upgrade to enter an early execution phase
- Verifying an initial early-phase executor job fails due to induced policy restriction
- Verifying retry job for that action is created and also fails
- Verifying threshold triggers early-phase abort and cluster returns to idle on original version


## `upgrade-strategies-creates-a-tlsroute-and-reports-healthy-79be45e7`

Path: `Upgrade Strategies > Gateway API > Gateway TLS Passthrough > creates a TLSRoute and reports healthy passthrough integration`

State: `active`

Covers: _none_

Labels: `upgrade`, `upgrades`, `cluster`, `slow`, `gateway`, `requires-gateway-api`, `tls-passthrough`

Recorded checkpoints:
- waiting for passthrough Gateway integration status
- verifying a TLSRoute is created for passthrough access
- verifying BackendTLSPolicy is not created for passthrough mode


## `upgrade-strategies-keeps-httproute-stable-and-switches-external-7a9e6dd5`

Path: `Upgrade Strategies > Gateway API > HTTPRoute Blue/Green Cutover > keeps HTTPRoute stable and switches external Service selector at cutover`

State: `active`

Covers: _none_

Labels: `upgrade`, `upgrades`, `cluster`, `slow`, `gateway`, `requires-gateway-api`, `bluegreen`, `e2e-anchor`

Recorded checkpoints:
- Capturing HTTPRoute before upgrade to verify stability
- Triggering upgrade
- Waiting for upgrade to progress to cutover phase
- Verifying external Service remains on Blue while DemotingBlue is in progress
- Waiting for Cleanup phase and verifying the external Service selector switches to Green
- Waiting for Blue/Green upgrade to complete
- Verifying legacy blue/green Services do not exist
- Verifying HTTPRoute remains stable throughout upgrade


## `upgrade-strategies-triggers-late-phase-rollback-after-promotion-c44db935`

Path: `Upgrade Strategies > Late-Phase Rollback Scenarios > triggers late-phase rollback after promotion failures and recovers when auth is restored`

State: `active`

Covers: _none_

Labels: `upgrade`, `upgrades`, `cluster`, `slow`, `failure`, `bluegreen`, `rollback`

Recorded checkpoints:
- Triggering a Blue/Green upgrade
- Waiting for Syncing hold before sending a promote request
- Verifying wait-green-synced completes before forcing promotion
- Introducing a realistic temporary auth misconfiguration and sending a promote request
- Verifying the promote request is recorded before promotion starts
- Verifying promotion executor job fails
- Verifying retry promotion job fails and triggers rollback threshold
- Restoring JWT auth role so rollback automation can proceed
- Verifying rollback was initiated
- Verifying rollback completes and cluster returns to stable initial version


## `upgrade-strategies-recovers-a-failed-rolling-upgrade-after-5a3c4b87`

Path: `Upgrade Strategies > Rolling Failure Recovery > recovers a failed rolling upgrade after a retry request clears stale state`

State: `active`

Covers: _none_

Labels: `upgrade`, `upgrades`, `cluster`, `slow`, `rolling`, `recovery`

Recorded checkpoints:
- Triggering a rolling upgrade with a bad non-semver image tag
- Waiting for the rolling upgrade to initialize
- Backdating upgrade start time to force the real timeout/retry path instead of waiting ten minutes
- Waiting for the rolling upgrade to fail with a retryable status
- Injecting a stale deterministic step-down job for the retry cleanup path
- Restoring the target image and requesting a rolling retry
- Verifying retry preparation clears failed status, records the handled request, and removes the stale step-down job
- Verifying the rolling upgrade resumes and completes successfully


## `upgrade-strategies-retries-a-failed-rolling-pre-upgrade-fade7ad8`

Path: `Upgrade Strategies > Rolling Snapshot Recovery > retries a failed rolling pre-upgrade snapshot before starting rollout`

State: `active`

Covers: _none_

Labels: `upgrade`, `upgrades`, `cluster`, `slow`, `rolling`, `snapshot`, `recovery`

Recorded checkpoints:
- Triggering a rolling upgrade with pre-upgrade snapshots enabled
- Waiting for the first pre-upgrade snapshot job to fail while rollout stays blocked
- Repairing the backup policy in OpenBao without changing the cluster generation
- Verifying the failed snapshot job is recycled and a fresh attempt succeeds
- Verifying the rolling upgrade proceeds only after the snapshot succeeds


## `upgrade-strategies-performs-rolling-upgrade-2302a23d`

Path: `Upgrade Strategies > Rolling Upgrade > performs rolling upgrade`

State: `active`

Covers: _none_

Labels: `upgrade`, `upgrades`, `cluster`, `slow`, `rolling`, `e2e-anchor`, `read-replicas`, `read-replicas-rolling`

Recorded checkpoints:
- Writing a secret before upgrade
- Triggering upgrade
- Verifying the steady read pool converges before voter rollout starts
- Monitoring rolling invariants during upgrade
- Verifying rolling step-down jobs are deterministic and successful
- Verifying secret persists after upgrade
- Verifying upgrade metrics reflect idle state


## `upgrade-strategies-acknowledges-break-glass-and-resumes-rollback-f99ce2fa`

Path: `Upgrade Strategies > Safe Mode (chaos) > acknowledges break glass and resumes rollback after the upgrade policy is repaired`

State: `active`

Covers: _none_

Labels: `upgrade`, `upgrades`, `cluster`, `slow`, `chaos`, `bluegreen`

Recorded checkpoints:
- Repairing the broken upgrade policy inside OpenBao
- Acknowledging break glass with the current nonce
- Verifying break glass deactivates and rollback resumes
- Verifying rollback completes and the cluster returns to the original version


## `upgrade-strategies-enters-safe-mode-when-rollback-consensus-e82ce327`

Path: `Upgrade Strategies > Safe Mode (chaos) > enters safe mode when rollback consensus repair job fails`

State: `active`

Covers: _none_

Labels: `upgrade`, `upgrades`, `cluster`, `slow`, `chaos`, `bluegreen`

Recorded checkpoints:
- Writing a secret before upgrade
- Triggering a Blue/Green upgrade
- Waiting for Green revision to be created
- Verifying Blue pods are still on initial version and Green pods are on target version
- Forcing rollback
- Waiting for rollback to start
- Finding the rollback consensus repair job
- Waiting for the rollback consensus repair job to fail
- Verifying secret persists in safe mode
- Asserting safe mode is set on the cluster


