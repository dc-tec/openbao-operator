# Manager Resilience

Source: `test/e2e/Manager_Resilience_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `manager-leader-failover` | fails over leader election and continues reconciling with a second controller replica | active | `leader-election`, `controller-failover`, `post-failover-reconcile` | `manager`, `cluster` |
| `manager-outage-adopts-existing-cluster` | reconciles an existing cluster after the controller is scaled down and back up | active | `controller-outage`, `existing-cluster-adoption`, `post-outage-reconcile` | `manager`, `cluster` |
| `manager-restart-idempotent-reconcile` | recovers idempotently when the controller restarts during initial and scale reconciliation | active | `controller-restart`, `idempotent-reconcile`, `scale-reconcile` | `manager`, `cluster` |

## `manager-leader-failover`

Path: `Manager Resilience > fails over leader election and continues reconciling with a second controller replica`

State: `active`

Generated fallback ID: `manager-resilience-fails-over-leader-election-and-continues-2baefc75`

Covers: `leader-election`, `controller-failover`, `post-failover-reconcile`

Labels: `manager`, `cluster`

Recorded checkpoints:
- scaling the controller deployment to two replicas
- capturing the current controller lease holder
- deleting the current leader pod to force failover
- waiting for a different controller pod to acquire leadership
- updating the cluster after failover and verifying reconciliation still works


## `manager-outage-adopts-existing-cluster`

Path: `Manager Resilience > reconciles an existing cluster after the controller is scaled down and back up`

State: `active`

Generated fallback ID: `manager-resilience-reconciles-an-existing-cluster-after-the-232e887a`

Covers: `controller-outage`, `existing-cluster-adoption`, `post-outage-reconcile`

Labels: `manager`, `cluster`

Recorded checkpoints:
- scaling the controller deployment to zero replicas
- changing cluster desired state while the controller is offline
- scaling the controller deployment back to one replica
- verifying the existing cluster is adopted and reconciled to the new desired state


## `manager-restart-idempotent-reconcile`

Path: `Manager Resilience > recovers idempotently when the controller restarts during initial and scale reconciliation`

State: `active`

Generated fallback ID: `manager-resilience-recovers-idempotently-when-the-controller-restarts-635ba796`

Covers: `controller-restart`, `idempotent-reconcile`, `scale-reconcile`

Labels: `manager`, `cluster`

Recorded checkpoints:
- restarting the controller while the initial reconcile is still in progress
- verifying the cluster still converges to a single ready StatefulSet
- scaling the cluster and restarting the controller during the reconcile
- verifying the scale reconcile finishes without duplicating managed resources
- reconfirming the cluster returns to an available steady state


