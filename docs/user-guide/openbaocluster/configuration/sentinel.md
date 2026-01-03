# Sentinel Drift Detection

The Sentinel is an optional, high-availability component that detects and corrects infrastructure drift in real-time.

!!! abstract "Goal"
    Sentinel watches managed resources (StatefulSets, Services, ConfigMaps) and triggers an immediate "Fast Path" reconciliation if unauthorized changes occur, bypassing slower operations like Upgrades or Backups.

## Configuration

Sentinel is configured via the `spec.sentinel` block.

=== "Basic"
    Enable Sentinel with default settings.

    ```yaml
    apiVersion: openbao.org/v1alpha1
    kind: OpenBaoCluster
    spec:
      sentinel:
        enabled: true
    ```

=== "Tuning"
    Adjust the **Debounce Window** to prevent "thundering herd" triggers during mass updates, or tune resource limits.

    ```yaml
    spec:
      sentinel:
        enabled: true
        debounceWindowSeconds: 5  # Wait 5s to coalesce events (Default: 2)
        resources:
          limits:
            memory: "256Mi"
            cpu: "500m"
    ```

=== "Advanced"
    Override the Sentinel image or disable it explicitly.

    ```yaml
    spec:
      sentinel:
        enabled: true
        image: "my-registry/sentinel:custom-tag"
    ```

## Security Model

Sentinel operates with strictly limited permissions to ensure cluster safety.

!!! security "Validating Admission Policy"
    A **ValidatingAdmissionPolicy** (`openbao-restrict-sentinel-mutations`) is required. It enforces that Sentinel can **only** patch specific status fields (`status.sentinel.*`). All other mutations (Spec, Labels, Secrets) are blocked at the API server level.

    This provides **mathematical security**: even if the Sentinel binary is compromised, it cannot escalate privileges.

## Operations

### Monitoring

Sentinel exposes a standard Kubernetes health endpoint.

- **Port**: `8080` (default)
- **Path**: `/healthz`
- **Success**: `200 OK`

### Troubleshooting

??? failure "Sentinel Not Triggering"
    If drift is not being detected:

    1. **Check Labels:** Resources must have `app.kubernetes.io/managed-by=openbao-operator`.
    2. **Check Actor Filtering:** Sentinel ignores changes made by the Operator itself. Verify the change was made by a different actor:
       ```bash
       kubectl get statefulset <name> -o jsonpath='{.metadata.managedFields[*].manager}'
       ```
    3. **Check Logs:**
       ```bash
       kubectl logs -l app.kubernetes.io/name=openbao-sentinel
       ```

??? failure "Triggering Too Often"
    If Sentinel fires constantly:

    1. **Increase Debounce:** Raise `debounceWindowSeconds` (e.g., to `10`).
    2. **Check Controllers:** Ensure no external controllers (e.g., HPA, Flux/Argo) are fighting the Operator for resource ownership without proper exclusions.

## Architecture

For a deep dive into the internal design, actor filtering logic, and fast-path reconciliation flow, see the [Sentinel Architecture](../../../architecture/sentinel.md) guide.
