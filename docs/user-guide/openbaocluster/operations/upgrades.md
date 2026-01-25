# Cluster Upgrades

The Operator supports two powerful upgrade strategies: **Rolling Update** (default) for efficiency, and **Blue/Green** for zero-downtime safety.

## One-Time Setup

To perform upgrades safely, the Operator uses a temporary "Upgrade Executor" job that requires permissions to talk to OpenBao.

### Prerequisite: Enable OIDC

The Upgrade Executor uses JWT Auth to authenticate. Ensure OIDC is enabled in your cluster:

```yaml
spec:
  selfInit:
    enabled: true
    oidc:
      enabled: true
```

The Operator automatically creates the necessary `sys/step-down` policies and JWT roles (`openbao-operator-upgrade`).

### Configure Executor

When OIDC is enabled, you can simply enable the upgrade strategy.

```yaml
spec:
  upgrade:
    # executorImage: inferred from operator version
    # jwtAuthRole: inferred (openbao-operator-upgrade)
```

## Executing Upgrades

To upgrade, simply update the `spec.version` field. The `updateStrategy` determines how this change is applied.

=== "Rolling Update (Default)"
    **Best for:** Standard upgrades, Dev/Test environments, Minimizing resource usage.

    The Operator updates pods one by one, ensuring the active leader steps down gracefully before termination to maintain availability.

    ```yaml
    spec:
      version: "2.4.4"
      updateStrategy:
        type: RollingUpdate
    ```

    **How it works:**
    1.  **Validation**: Checks if the new version is valid.
    2.  **Snapshot** (Optional): Takes a pre-upgrade backup.
    3.  **Rolling Replace**: Updates Pod 0 -> Pod 1 -> Pod 2.
    4.  **Leader Handling**: If updating the active leader, triggers `sys/step-down` first.

=== "Blue/Green (Zero Downtime)"
    **Best for:** Production critical paths, Major version jumps, Instant rollback capability.

    The Operator spins up a **parallel** "Green" cluster, syncs data, validates it, and then switches traffic over atomically.

    ```mermaid
    flowchart TB
        Start[Start Upgrade]
        
        subgraph Blue["Blue Revision (Current)"]
            B[Active Cluster]
        end

        subgraph Green["Green Revision (New)"]
            direction TB
            Deploy[1. Deploy Green Pods]
            Sync[2. Sync Data form Blue]
            Test[3. Run Verification]
        end

        Start --> Deploy
        Deploy --> Sync
        Sync --> Test
        Test -- "Success" --> Switch[4. Switch Traffic to Green]
        Switch --> Cleanup[5. Delete Blue Cluster]

        style Blue fill:transparent,stroke:#2979ff,stroke-width:2px
        style Green fill:transparent,stroke:#00e676,stroke-width:2px,color:#fff
        style Switch fill:transparent,stroke:#ffa726,stroke-width:2px
    ```

    **Configuration:**

    ```yaml
    spec:
      version: "2.4.4"
      updateStrategy:
        type: BlueGreen
        blueGreen:
          autoPromote: true          # Automatically switch traffic if healthy
          preUpgradeSnapshot: true   # Backup before starting
          autoRollback:
            enabled: true            # Revert if Green fails validation
    ```

## Advanced Upgrade Options

### Verification Hooks

Run a custom container to "smoke test" the Green cluster before cutover.

```yaml
spec:
  updateStrategy:
    blueGreen:
      verification:
        prePromotionHook:
          image: curlimages/curl
          command: ["curl", "-f", "https://green-cluster:8200/v1/sys/health"]
```

### Auto-Rollback

If the Green cluster fails validation or upgrade jobs fail during the early upgrade phases, the Operator can automatically roll back.

```yaml
spec:
  updateStrategy:
    blueGreen:
      autoRollback:
        enabled: true
        onJobFailure: true
        onValidationFailure: true
```

### Gateway API and Blue/Green upgrades

When using **Gateway API**, the Operator creates an `HTTPRoute` that targets the cluster's main external Service (`<cluster>-public`). During cutover, the operator updates that Service's selector to point at the Green revision.

```yaml
spec:
  gateway:
    enabled: true
    hostname: bao.example.com
    gatewayRef:
      name: main-gateway
  updateStrategy:
    type: BlueGreen
    blueGreen:
      autoPromote: true
```

### Monitoring Progress

Track the upgrade status directly on the CR:

```sh
kubectl get openbaocluster my-cluster -o jsonpath='{.status.blueGreen}'
```
