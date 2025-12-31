# Sentinel Drift Detection

The Sentinel is an optional per-cluster sidecar controller that provides high-availability drift detection and fast-path reconciliation. When enabled, the Sentinel watches for unauthorized changes to managed infrastructure resources and immediately triggers the operator to correct them.

## Overview

The Sentinel addresses a critical gap in Kubernetes operators: **detecting and correcting infrastructure drift in real-time**. While the operator reconciles based on `OpenBaoCluster.Spec`, there's a window where manual changes or external actors could modify managed resources (StatefulSets, Services, ConfigMaps) before the next reconciliation cycle.

**Key Benefits:**

- **Fast Drift Correction:** Detects unauthorized changes within seconds and triggers immediate reconciliation
- **High Availability:** Runs as a separate Deployment, independent of the operator controller
- **Security:** ValidatingAdmissionPolicy (VAP) prevents privilege escalation even if the Sentinel binary is compromised
- **Efficiency:** Fast-path mode skips expensive operations (upgrades, backups) during drift correction

## Configuration

The Sentinel is configured via `spec.sentinel` in the `OpenBaoCluster` resource:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: my-cluster
  namespace: default
spec:
  profile: Development
  # ... other spec fields ...
  sentinel:
    enabled: true  # Enable Sentinel (default: true when sentinel is set)
    image: ""      # Optional: Override Sentinel image (defaults to openbao/operator-sentinel:<operator-version>)
    resources:     # Optional: Override resource limits
      requests:
        memory: "64Mi"
        cpu: "100m"
      limits:
        memory: "128Mi"
        cpu: "200m"
    debounceWindowSeconds: 2  # Optional: Debounce window in seconds (default: 2, range: 1-60)
```

### Configuration Options

#### `enabled`

Controls whether the Sentinel is deployed. Defaults to `true` when `spec.sentinel` is set.

```yaml
spec:
  sentinel:
    enabled: false  # Disable Sentinel
```

#### `image`

Allows overriding the Sentinel container image. If not specified, defaults to `openbao/operator-sentinel:<operator-version>` where `<operator-version>` is derived from the `OPERATOR_VERSION` environment variable in the operator Deployment.

```yaml
spec:
  sentinel:
    image: "my-registry/operator-sentinel:v1.0.0"  # Custom image
```

**Note:** If image verification is enabled (`spec.imageVerification.enabled`), the Sentinel image will be verified using the same configuration as the main OpenBao image.

#### `resources`

Configures resource limits for the Sentinel container. Defaults to:

```yaml
resources:
  requests:
    memory: "64Mi"
    cpu: "100m"
  limits:
    memory: "128Mi"
    cpu: "200m"
```

You can override these defaults:

```yaml
spec:
  sentinel:
    resources:
      requests:
        memory: "128Mi"
        cpu: "200m"
      limits:
        memory: "256Mi"
        cpu: "500m"
```

#### `debounceWindowSeconds`

Controls the debounce window for drift detection. Multiple drift events within this window are coalesced into a single trigger to prevent "thundering herd" scenarios.

- **Default:** `2` seconds
- **Range:** `1` to `60` seconds
- **Use Case:** Increase this value if you expect rapid, simultaneous changes to multiple resources (e.g., during node failures)

```yaml
spec:
  sentinel:
    debounceWindowSeconds: 5  # Wait 5 seconds before triggering
```

## How It Works

### 1. Deployment

When `spec.sentinel.enabled` is `true`, the operator creates:

- **ServiceAccount:** `openbao-sentinel` in the cluster namespace
- **Role:** `openbao-sentinel-role` with read-only access to infrastructure and limited patch access to `OpenBaoCluster`
- **RoleBinding:** Binds the ServiceAccount to the Role
- **Deployment:** Single-replica Deployment running the Sentinel binary

### 2. Watching

The Sentinel watches for changes to:

- **StatefulSets** labeled with `app.kubernetes.io/managed-by=openbao-operator` and `app.kubernetes.io/instance=<cluster-name>`
- **Services** with the same labels
- **ConfigMaps** with the same labels

### 3. Actor Filtering

To prevent infinite loops, the Sentinel inspects `object.metadata.managedFields` and ignores changes made by the operator itself. This ensures that when the operator updates a resource (e.g., during normal reconciliation), the Sentinel doesn't immediately trigger another reconciliation.

### 4. Debouncing

Multiple drift events within the debounce window are coalesced into a single trigger. For example, if a node fails and 50 pods change state simultaneously, the Sentinel will fire only one trigger after the debounce window expires.

### 5. Scope and Secret Safety

The Sentinel does **not** have access to Secrets. RBAC for the Sentinel is strictly limited to read-only access for StatefulSets, Services, and ConfigMaps, plus patch access on `OpenBaoCluster` resources for drift triggers. Drift that involves unseal keys or root tokens is detected indirectly via OpenBao health and status, not by inspecting Secret data.

### 6. Trigger

When drift is detected, the Sentinel patches the `OpenBaoCluster` with:

```yaml
annotations:
  openbao.org/sentinel-trigger: "2025-01-15T10:30:45.123456789Z"
  openbao.org/sentinel-trigger-resource: "StatefulSet/my-cluster"
```

These annotations are the **only** metadata mutations the Sentinel is authorized to make (enforced by ValidatingAdmissionPolicy).

### 7. Fast Path

The operator detects the trigger annotation and enters "fast path" mode:

- **CertManager** and **InfrastructureManager** run normally (to fix the drift)
- **InitManager** runs if needed
- **UpgradeManager** and **BackupManager** are **skipped** to minimize latency

After successful reconciliation, the operator clears the trigger annotation, which causes a second reconciliation (normal path) to ensure full consistency.

## Security

### ValidatingAdmissionPolicy

A cluster-scoped ValidatingAdmissionPolicy (`openbao-restrict-sentinel-mutations`) enforces that the Sentinel can only modify the drift trigger annotations. All other mutations are blocked at the API server level:

- **Spec changes:** Blocked
- **Status changes:** Blocked
- **Other annotations (besides `openbao.org/sentinel-trigger` and `openbao.org/sentinel-trigger-resource`):** Blocked
- **Labels and finalizers:** Blocked

This provides **mathematical security**: even if the Sentinel binary is compromised, it cannot escalate privileges or modify cluster configuration.

**Required dependency:** These admission policies are required. If the operator cannot verify that the Sentinel mutation policy (and its binding) is installed and correctly bound, it will treat Sentinel as unsafe and will not deploy the Sentinel Deployment, and Sentinel-triggered fast-path is disabled. The supported Kubernetes baseline for these controls is v1.33+.

**Note on names:** When deploying via `config/default`, `kustomize` applies a `namePrefix` (for example, `openbao-operator-`) to cluster-scoped admission resources.

### RBAC

The Sentinel has minimal permissions:

- **Read-only access** to infrastructure resources (`get`, `list`, `watch`)
- **Limited patch access** to `OpenBaoCluster` resources (`get`, `patch`)

The Sentinel cannot:

- Create or delete resources
- Modify Spec or Status
- Access resources outside the cluster namespace
- Modify labels or finalizers

## Monitoring

The Sentinel exposes a health endpoint at `/healthz` for Kubernetes liveness and readiness probes. The endpoint returns:

- **200 OK:** Sentinel is ready and watching
- **503 Service Unavailable:** Sentinel is not ready (e.g., manager not started, watches not established)

## Troubleshooting

### Sentinel Not Triggering

If the Sentinel is not detecting drift:

1. **Check Deployment Status:**

   ```bash
   kubectl get deployment <cluster-name>-sentinel -n <namespace>
   kubectl logs deployment/<cluster-name>-sentinel -n <namespace>
   ```

2. **Verify Labels:** Ensure managed resources have the correct labels:

   ```bash
   kubectl get statefulset <cluster-name> -n <namespace> --show-labels
   ```

   Should show: `app.kubernetes.io/managed-by=openbao-operator` and `app.kubernetes.io/instance=<cluster-name>`

3. **Check Actor Filtering:** The Sentinel ignores operator updates. Verify the change was made by a different actor:

   ```bash
   kubectl get statefulset <cluster-name> -n <namespace> -o jsonpath='{.metadata.managedFields[*].manager}'
   ```

### Sentinel Triggering Too Often

If the Sentinel is triggering too frequently:

1. **Increase Debounce Window:**

   ```yaml
   spec:
     sentinel:
       debounceWindowSeconds: 10  # Increase from default 2 seconds
   ```

2. **Check for Legitimate Drift:** Verify that resources are actually being modified by unauthorized actors (not just metadata updates).

### Sentinel Not Deploying

If the Sentinel Deployment is not created:

1. **Check Spec:**

   ```bash
   kubectl get openbaocluster <cluster-name> -n <namespace> -o yaml | grep -A 10 sentinel
   ```

2. **Check Operator Logs:**

   ```bash
   kubectl logs deployment/openbao-operator-controller -n openbao-operator-system | grep -i sentinel
   ```

3. **Verify Image:** If image verification is enabled, ensure the Sentinel image is signed with the same key/identity as the main OpenBao image.

## Disabling Sentinel

To disable the Sentinel:

```yaml
spec:
  sentinel:
    enabled: false
```

Or remove the `sentinel` field entirely:

```yaml
spec:
  # sentinel field removed
```

The operator will clean up the Sentinel Deployment, ServiceAccount, Role, and RoleBinding when Sentinel is disabled.

## Best Practices

1. **Enable Sentinel in Production:** Sentinel provides critical drift detection and should be enabled for production clusters.

2. **Use Image Verification:** If image verification is enabled for the main OpenBao image, ensure the Sentinel image is signed with the same key/identity.

3. **Monitor Sentinel Health:** Add alerts for Sentinel Deployment failures or health probe failures.

4. **Tune Debounce Window:** Adjust `debounceWindowSeconds` based on your cluster's change patterns. Higher values reduce trigger frequency but increase detection latency.

5. **Resource Limits:** The default resource limits are conservative. Monitor Sentinel resource usage and adjust if needed.

## See Also

- [Architecture: Sentinel](../architecture/sentinel.md)
- [Security: Admission Policies](../security/admission-policies.md)
- [Advanced Configuration](advanced-configuration.md)
