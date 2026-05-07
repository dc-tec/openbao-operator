# Cluster PKCS#11 Unseal

Source: `test/e2e/Cluster_Unseal_PKCS11_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `cluster-unseal-pkcs11-initializes-restarts-and-scales-using-a-50e1c83b` | initializes, restarts, and scales using a SoftHSM-backed PKCS#11 seal | active | _none_ | `cluster`, `lifecycle`, `unseal`, `pkcs11`, `hsm` |

## `cluster-unseal-pkcs11-initializes-restarts-and-scales-using-a-50e1c83b`

Path: `Cluster PKCS#11 Unseal > initializes, restarts, and scales using a SoftHSM-backed PKCS#11 seal`

State: `active`

Covers: _none_

Labels: `cluster`, `lifecycle`, `unseal`, `pkcs11`, `hsm`

Recorded checkpoints:
- creating the PKCS#11 credentials Secret
- creating an OpenBaoCluster configured for PKCS#11 unseal
- waiting for the initial PKCS#11-sealed pod to become ready
- verifying the rendered OpenBao config contains the PKCS#11 seal stanza
- verifying the StatefulSet uses the PKCS#11 runtime env wiring
- deleting the pod and validating it auto-unseals after restart
- scaling up to verify new pods can use the seeded PKCS#11 key material
- scaling back down cleanly
