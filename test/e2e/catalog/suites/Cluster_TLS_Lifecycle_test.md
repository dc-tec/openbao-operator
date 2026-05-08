# Cluster TLS Lifecycle

Source: `test/e2e/Cluster_TLS_Lifecycle_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `tls-lifecycle-server-secret-regeneration` | verifies operator-managed TLS with the cluster CA and regenerates the server Secret | active | `tls-lifecycle`, `tls-verification`, `secret-regeneration`, `cert-replacement`, `tls-hot-reload`, `pod-stability` | `tls`, `cluster`, `lifecycle` |

## `tls-lifecycle-server-secret-regeneration`

Path: `Cluster TLS Lifecycle > verifies operator-managed TLS with the cluster CA and regenerates the server Secret`

State: `active`

Generated fallback ID: `cluster-tls-lifecycle-verifies-operator-managed-tls-with-the-6dd4a6e5`

Covers: `tls-lifecycle`, `tls-verification`, `secret-regeneration`, `cert-replacement`, `tls-hot-reload`, `pod-stability`

Labels: `tls`, `cluster`, `lifecycle`

Recorded checkpoints:
- waiting for the cluster to become Available with TLS ready
- waiting for the TLS Secrets to exist
- writing a secret through the JWT-authenticated test role
- reading the secret over TLS with explicit CA validation
- recording the initial certificate and pod reload state
- deleting the managed tls-server Secret as the operator controller
- verifying the OpenBao pod stays ready without being recreated while the secret is reissued
- triggering reconcile and waiting for the tls-server Secret to be reissued
- verifying the pod receives a new TLS reload hash without restarting
- reconfirming cluster readiness and stability after server Secret regeneration
- re-reading the secret over TLS with CA validation after regeneration
