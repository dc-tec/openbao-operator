# InitManager (Cluster Initialization)

**Responsibility:** Automate initial cluster initialization and root token management.

The InitManager handles the bootstrapping of a new OpenBao cluster, including initial Raft leader election and storing the root token securely.

## Initialization Workflow

1. **Single-Pod Bootstrap:** During initial cluster creation, the InfrastructureManager starts with 1 replica (regardless of `spec.replicas`) until initialization completes.

2. **Wait for Container:** The InitManager waits for the pod-0 container to be running (not necessarily ready, since the readiness probe may fail until OpenBao is initialized).

3. **Check Status (Preferred):** Read OpenBao's Kubernetes service registration labels on pod-0 (`openbao-initialized`, `openbao-sealed`).

4. **Check Status (Fallback):** If labels are not yet available, query OpenBao via the HTTP health endpoint (`GET /v1/sys/health`) using the per-cluster TLS CA.

5. **Initialize:** If not initialized, call the HTTP initialization endpoint (`PUT /v1/sys/init`) against pod-0 using the Operator's in-cluster OpenBao client (no `pods/exec` or CLI dependencies).

6. **Store Root Token:** Parse the initialization response and store the root token in a per-cluster Secret (`<cluster>-root-token`).

7. **Mark Initialized:** Set `Status.Initialized = true` to signal the InfrastructureManager to scale up to the desired replica count.

## Reconciliation Semantics

- The InitManager only runs when `Status.Initialized == false`.
- Once `Status.Initialized` is true, the InitManager short-circuits and performs no operations.
- Errors during initialization (e.g., network issues, pod not ready) cause requeues with backoff; the cluster remains at 1 replica until successful initialization.

!!! warning "Security"
    The initialization response (which contains sensitive unseal keys and root token) is NEVER logged.

See also: [Secrets Management](../security/secrets-management.md)
