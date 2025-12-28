# Sentinel Drift Detection (Day N)

The Sentinel is an optional per-cluster sidecar controller that provides high-availability drift detection and fast-path reconciliation. When enabled via `spec.sentinel.enabled`, the operator deploys a lightweight Deployment that watches for unauthorized changes to managed infrastructure resources.

## How It Works

1. **Deployment:** The operator creates a Sentinel Deployment in the cluster namespace with a single replica (stateless).
2. **Watching:** The Sentinel watches StatefulSets, Services, ConfigMaps, and Secrets that are labeled with `app.kubernetes.io/managed-by=openbao-operator` and `app.kubernetes.io/instance=<cluster-name>`.
3. **Actor Filtering:** The Sentinel ignores changes made by the operator itself by inspecting `managedFields` to prevent infinite loops.
4. **Debouncing:** Multiple drift events within a configurable window (default: 2 seconds) are coalesced into a single trigger to prevent "thundering herd" scenarios.
5. **Secret Safety:** For unseal keys and root tokens, the Sentinel uses SHA256 hash comparison to avoid false positives from metadata-only updates.
6. **Trigger:** When drift is detected, the Sentinel patches the `OpenBaoCluster` with the annotation `openbao.org/sentinel-trigger: <timestamp>`.
7. **Fast Path:** The operator detects this annotation and enters "fast path" mode, skipping expensive operations (Upgrade and Backup managers) to quickly correct the drift.
8. **Cleanup:** After successful reconciliation, the operator clears the trigger annotation, which causes a second reconciliation (normal path) to ensure full consistency including backups/upgrades.

## Security Model

- The Sentinel has read-only access to infrastructure resources and limited patch access to `OpenBaoCluster` resources.
- A ValidatingAdmissionPolicy (VAP) enforces that the Sentinel can only modify the trigger annotation; all other mutations (Spec, Status, other annotations, labels, finalizers) are blocked at the API server level.
- This mathematically prevents privilege escalation even if the Sentinel binary is compromised.

## Configuration

- `spec.sentinel.enabled`: Enable/disable Sentinel (default: `true` when `spec.sentinel` is set).
- `spec.sentinel.image`: Override the Sentinel container image (defaults to `openbao/operator-sentinel:<operator-version>`).
- `spec.sentinel.resources`: Configure resource limits (defaults: 64Mi memory, 100m CPU requests; 128Mi memory, 200m CPU limits).
- `spec.sentinel.debounceWindowSeconds`: Configure debounce window (default: 2 seconds, range: 1-60).

**Note:** The Sentinel is designed to be a lightweight, stateless component. It does not store state and can be safely restarted or recreated without affecting cluster operation.
