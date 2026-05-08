# Cluster KMIP Unseal

Source: `test/e2e/Cluster_Unseal_KMIP_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `cluster-unseal-kmip-initializes-restarts-and-scales-using-a-d2fcee95` | initializes, restarts, and scales using a PyKMIP-backed KMIP seal | active | _none_ | `cluster`, `lifecycle`, `unseal`, `kmip`, `hsm` |

## `cluster-unseal-kmip-initializes-restarts-and-scales-using-a-d2fcee95`

Path: `Cluster KMIP Unseal > initializes, restarts, and scales using a PyKMIP-backed KMIP seal`

State: `active`

Covers: _none_

Labels: `cluster`, `lifecycle`, `unseal`, `kmip`, `hsm`

Recorded checkpoints:
- creating KMIP mTLS material
- starting the PyKMIP fixture server
- creating an OpenBaoCluster configured for KMIP unseal
- waiting for the initial KMIP-sealed pod to become ready
- verifying the rendered OpenBao config contains the KMIP seal stanza
- verifying the StatefulSet projects KMIP credential files
- deleting the pod and validating it auto-unseals after restart
- scaling up to verify new pods can use the seeded KMIP key material
- scaling back down cleanly
