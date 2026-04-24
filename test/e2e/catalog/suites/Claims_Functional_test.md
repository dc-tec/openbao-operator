# Claims Functional

Source: `test/e2e/Claims_Functional_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `claims-functional-restore-request` | executes a claim restore request from a selected completed backup request | active | `claim-restore-request`, `claim-restore-backup-request-source`, `claim-restore-status-projection` | `claims`, `claims-functional` |
| `claims-functional-backup-request` | executes a manual claim backup request and projects the result onto claim status | active | `claim-backup-request`, `claim-backup-status-projection` | `claims`, `claims-functional` |
| `claims-functional-upgrade-request` | executes an in-place claim upgrade request against a new service-profile revision | active | `claim-upgrade-request`, `claim-upgrade-rollout` | `claims`, `claims-functional`, `claims-upgrade` |
| `claims-functional-missing-bootstrap-source` | keeps the claim pending when a secret-backed bootstrap source is missing | active | `claim-missing-bootstrap-source` | `claims`, `claims-functional`, `negative` |
| `claims-functional-catalog-profiles` | projects production catalog profiles into the claim-managed cluster | active | `claim-catalog-runtime-profile`, `claim-catalog-network-profile`, `claim-catalog-upgrade-policy`, `claim-catalog-read-replica-profile` | `claims`, `claims-functional` |
| `claims-functional-gateway` | publishes the external gateway hostname once gateway integration is ready | active | `claim-gateway`, `claim-external-endpoint-publication` | `claims`, `claims-functional`, `requires-gateway-api` |
| `claims-functional-ingress` | publishes the external ingress hostname once ingress integration is ready | active | `claim-ingress`, `claim-external-endpoint-publication` | `claims`, `claims-functional` |

## `claims-functional-restore-request`

Path: `Claims Functional > executes a claim restore request from a selected completed backup request`

State: `active`

Generated fallback ID: `claims-functional-executes-a-claim-restore-request-from-c61d9ae9`

Covers: `claim-restore-request`, `claim-restore-backup-request-source`, `claim-restore-status-projection`

Labels: `claims`, `claims-functional`

Recorded checkpoints:
- creating a fresh successful backup for the restore request to consume
- surfacing the completed claim backup in the namespaced backup request inventory
- waiting for the restore request to start or complete
- publishing the active restore workflow on claim status
- waiting for the restore request to complete successfully
- clearing the active restore workflow from claim status after completion


## `claims-functional-backup-request`

Path: `Claims Functional > executes a manual claim backup request and projects the result onto claim status`

State: `active`

Generated fallback ID: `claims-functional-executes-a-manual-claim-backup-request-0cd71fcc`

Covers: `claim-backup-request`, `claim-backup-status-projection`

Labels: `claims`, `claims-functional`

Recorded checkpoints:
- waiting for the claim to publish the active backup workflow summary
- waiting for the backup request to complete successfully
- surfacing the completed claim backup in the namespaced backup request inventory
- projecting the completed backup onto claim status and clearing the workflow summary


## `claims-functional-upgrade-request`

Path: `Claims Functional > executes an in-place claim upgrade request against a new service-profile revision`

State: `active`

Generated fallback ID: `claims-functional-executes-an-in-place-claim-upgrade-9594909c`

Covers: `claim-upgrade-request`, `claim-upgrade-rollout`

Labels: `claims`, `claims-functional`, `claims-upgrade`

Recorded checkpoints:
- publishing the next immutable service-profile revision on the same offering alias
- waiting for the request to enter rollout
- waiting for the claim to publish an active maintenance summary
- waiting for the claim to converge onto the upgraded revision
- waiting for the upgrade request to complete and the claim workflow summary to clear
- projecting the upgraded backup schedule onto the local OpenBaoCluster


## `claims-functional-missing-bootstrap-source`

Path: `Claims Functional > keeps the claim pending when a secret-backed bootstrap source is missing`

State: `active`

Generated fallback ID: `claims-functional-keeps-the-claim-pending-when-a-e6a05016`

Covers: `claim-missing-bootstrap-source`

Labels: `claims`, `claims-functional`, `negative`


## `claims-functional-catalog-profiles`

Path: `Claims Functional > projects production catalog profiles into the claim-managed cluster`

State: `active`

Generated fallback ID: `claims-functional-projects-production-catalog-profiles-into-the-8b3e81aa`

Covers: `claim-catalog-runtime-profile`, `claim-catalog-network-profile`, `claim-catalog-upgrade-policy`, `claim-catalog-read-replica-profile`

Labels: `claims`, `claims-functional`


## `claims-functional-gateway`

Path: `Claims Functional > publishes the external gateway hostname once gateway integration is ready`

State: `active`

Generated fallback ID: `claims-functional-publishes-the-external-gateway-hostname-once-265ffa07`

Covers: `claim-gateway`, `claim-external-endpoint-publication`

Labels: `claims`, `claims-functional`, `requires-gateway-api`

Recorded checkpoints:
- marking the referenced Gateway as programmed


## `claims-functional-ingress`

Path: `Claims Functional > publishes the external ingress hostname once ingress integration is ready`

State: `active`

Generated fallback ID: `claims-functional-publishes-the-external-ingress-hostname-once-5c177257`

Covers: `claim-ingress`, `claim-external-endpoint-publication`

Labels: `claims`, `claims-functional`

Recorded checkpoints:
- publishing a load balancer address on the managed Ingress
