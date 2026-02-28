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
    # image: inferred from operator version
    # jwtAuthRole: inferred (openbao-operator-upgrade)
```

## Executing Upgrades

To upgrade, update `spec.version`. The strategy configured in `spec.upgrade.strategy` determines how this change is applied.

=== "Rolling Update (Default)"
    **Best for:** Standard upgrades, Dev/Test environments, Minimizing resource usage.

    The Operator updates pods one by one, ensuring the active leader steps down gracefully before termination to maintain availability.

    ```yaml
    spec:
      version: "2.4.4"
      upgrade:
        strategy: RollingUpdate
    ```

    **How it works:**
    1.  **Validation**: Checks if the new version is valid.
    2.  **Snapshot** (Optional): Takes a pre-upgrade backup.
    3.  **Partitioned Rollout**: Locks StatefulSet partition, then updates pods in reverse ordinal order (for example, `2 -> 1 -> 0`).
    4.  **Leader Handling**: If the target pod is leader, runs a `sys/step-down` executor job before restart.
    5.  **Convergence Gate**: Finalizes only after all pods are updated, Ready, and healthy.

    !!! note
        You can see multiple step-down Jobs during one rolling upgrade when leadership moves between different target pods. This is expected.

=== "Blue/Green (Zero Downtime)"
    **Best for:** Production-critical paths and major version upgrades where controlled cutover is required.

    The OpenBao Operator creates a **parallel** Green revision, syncs and validates it, promotes Green to voters, then shifts traffic during `Cleanup`.

    ```mermaid
    flowchart TB
        Start[Start Upgrade]
        
        subgraph Blue["Blue Revision (Current)"]
            B[Active Cluster]
        end

        subgraph Green["Green Revision (New)"]
            direction TB
            Deploy[1. Deploy Green Pods]
            Sync[2. Sync Data from Blue]
            Test[3. Run Verification]
        end

        Start --> Deploy
        Deploy --> Sync
        Sync --> Test
        Test -- "Success" --> Promote[4. Promote Green Voters]
        Promote --> Demote[5. Demote Blue Non-Voters]
        Demote --> Switch[6. Cleanup Phase: Switch Traffic to Green]
        Switch --> Cleanup[7. Remove Blue Peers and Delete Blue Cluster]

        classDef read fill:transparent,stroke:#60a5fa,stroke-width:2px,color:#fff;
        classDef write fill:transparent,stroke:#22c55e,stroke-width:2px,color:#fff;
        classDef process fill:transparent,stroke:#9333ea,stroke-width:2px,color:#fff;

        class Start read;
        class B write;
        class Deploy,Sync,Test,Promote,Demote,Switch,Cleanup process;
    ```

    **Blue/Green phases (`status.blueGreen.phase`):**
    1. `DeployingGreen`
    2. `JoiningMesh`
    3. `Syncing`
    4. `Promoting`
    5. `DemotingBlue`
    6. `Cleanup`

    **Configuration:**

    ```yaml
    spec:
      version: "2.4.4"
      upgrade:
        strategy: BlueGreen
        preUpgradeSnapshot: true  # Optional: backup before starting (requires spec.backup)
        blueGreen:
          autoPromote: true  # Automatically switch traffic if healthy
          autoRollback:
            enabled: true  # Abort early failures, rollback late failures
    ```

## Advanced Upgrade Options

### Verification Hooks

Run a custom container to "smoke test" the Green cluster before cutover.

```yaml
spec:
  upgrade:
    strategy: BlueGreen
    blueGreen:
      verification:
        prePromotionHook:
          image: curlimages/curl
          command: ["curl", "-f", "https://green-cluster:8200/v1/sys/health"]
```

### Auto-Rollback

If Green validation fails or executor jobs repeatedly fail, the OpenBao Operator can automatically recover:

- In early phases (`DeployingGreen`, `JoiningMesh`, `Syncing`), it aborts the upgrade and removes Green.
- In later phases (`Promoting`, `DemotingBlue`, `Cleanup`), it triggers rollback (`RollingBack`, `RollbackCleanup`).
- After Blue has been fully removed in `Cleanup`, rollback is no longer possible.

```yaml
spec:
  upgrade:
    strategy: BlueGreen
    blueGreen:
      autoRollback:
        enabled: true
        onJobFailure: true
        onValidationFailure: true
```

### Gateway API and Blue/Green upgrades

When using **Gateway API**, the OpenBao Operator creates an `HTTPRoute` that targets the cluster's main external Service (`<cluster>-public`). During `Cleanup`, it updates that Service selector to the Green revision.

```yaml
spec:
  gateway:
    enabled: true
    hostname: bao.example.com
    gatewayRef:
      name: main-gateway
  upgrade:
    strategy: BlueGreen
    blueGreen:
      autoPromote: true
```

### Monitoring Progress

Track upgrade status directly on the CR:

=== "Rolling Update"

    ```sh
    kubectl get openbaocluster my-cluster -o jsonpath='{.status.currentVersion}{"\n"}{.status.upgrade}{"\n"}'
    ```

=== "Blue/Green"

    ```sh
    kubectl get openbaocluster my-cluster -o jsonpath='{.status.blueGreen.phase}{"\n"}{.status.blueGreen.jobFailureCount}{"\n"}{.status.blueGreen.lastJobFailure}{"\n"}'
    ```

## Official OpenBao Documentation

- [Upgrade Guide](https://openbao.org/docs/upgrading/)
- [HA Upgrade Guidance](https://openbao.org/docs/upgrading/ha-upgrade/)
- [Operator Step-Down Command](https://openbao.org/docs/commands/operator/step-down/)
- [Operator Raft Command](https://openbao.org/docs/commands/operator/raft/)
