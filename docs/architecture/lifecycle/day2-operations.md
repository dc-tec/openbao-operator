# Day 2: Operations & Upgrades

Day 2 operations cover the ongoing management of the cluster, including version upgrades and maintenance.

!!! tip "User Guide"
    See the [Upgrade Guide](../../user-guide/openbaocluster/operations/upgrades.md) for detailed upgrade strategies (Rolling vs Blue/Green).

## Cluster Operations / Upgrades

## Cluster Operations / Upgrades

=== "Rolling Update (Default)"

    1. User configures upgrade executor:
       - Set `spec.upgrade.executorImage` (container image used by upgrade Jobs)
       - Set `spec.upgrade.jwtAuthRole` and configure the role in OpenBao (binds to `<cluster-name>-upgrade-serviceaccount`, automatically created by operator)
    2. User updates `OpenBaoCluster.Spec.Version` and/or `Spec.Image`.
    3. Upgrade Manager (adminops controller) detects version drift and performs pre-upgrade validation:
       - Validates semantic versioning (blocks downgrades by default).
       - Verifies all pods are Ready and quorum is healthy.
       - Optionally triggers a pre-upgrade backup if `spec.upgrade.preUpgradeSnapshot` is enabled.
    4. Upgrade Manager orchestrates Raft-aware rolling updates:
       - Locks StatefulSet updates using partitioning.
       - Iterates pods in reverse ordinal order.
       - Runs an upgrade Job to perform leader step-down before updating the leader pod.
       - Waits for pod Ready, OpenBao health, and Raft sync after each update.
    5. Upgrade progress is persisted in `Status.Upgrade`, allowing resumption after Operator restart.
    6. On completion, `Status.CurrentVersion` is updated and `Status.Upgrade` is cleared.

    !!! note "Upgrade Policy"
        Upgrades are designed to be safe and resumable. Downgrades are blocked by default. If an upgrade fails, it halts and sets `Degraded=True`; automated rollback is not supported. Root tokens are not used for upgrade operations.

    ### Sequence Diagram (Rolling Updates)

    ```mermaid
    sequenceDiagram
        autonumber
        participant U as User
        participant K as Kubernetes API
        participant Op as OpenBao Operator
        participant Bao as OpenBao Pods

        U->>K: Patch OpenBaoCluster.spec.version
        K-->>Op: Watch OpenBaoCluster (version drift)
        Op->>Op: Validate versions, health, optional pre-upgrade backup
        Op->>K: Patch StatefulSet updateStrategy (lock with partition)
        loop per pod (highest ordinal -> 0)
            Op->>Bao: /v1/sys/health on target pod
            alt pod is leader
                Op->>Bao: /v1/sys/step-down
            end
            Op->>K: Decrement StatefulSet.partition to update pod
            K-->>Bao: Roll new pod
            Bao-->>Op: Pod Ready + OpenBao health OK
        end
        Op->>K: Update OpenBaoCluster.status.currentVersion
        Op->>K: Clear OpenBaoCluster.status.upgrade
    ```

=== "Blue/Green Upgrade"

    Blue/Green upgrades provide zero-downtime updates by creating a parallel "Green" standby cluster.

    1.  **Drift Detection:** User updates `OpenBaoCluster` spec with a new version or image, using the Blue/Green strategy.
    2.  **Green Creation:** The operator creates a new "Green" StatefulSet with the new version.
    3.  **Join & Standby:** Green pods start and join the existing "Blue" Raft cluster as non-voters (or voters, depending on strategy). They replicate data but do not serve traffic.
    4.  **Health Check:** Operator verifies the Green cluster is healthy and fully replicated.
    5.  **Cutover:** Operator updates the Service selector to point to the Green pods. Traffic switches instantly.
    6.  **Cleanup:** After a verification period or manual confirmation, the old "Blue" StatefulSet is scaled down and terminated.

    ### Sequence Diagram (Blue/Green)

    ```mermaid
    sequenceDiagram
        autonumber
        participant U as User
        participant K as Kubernetes API
        participant Op as OpenBao Operator
        participant Blue as Blue Pods (v1)
        participant Green as Green Pods (v2)

        U->>K: Update Image to v2 (BlueGreen Strategy)
        K-->>Op: Watch OpenBaoCluster
        Op->>K: Create Green StatefulSet (v2)
        K-->>Green: Start Green Pods
        Green->>Blue: Join Raft Cluster (Standby)
        Op->>Green: Wait for Healthy
        Op->>K: Switch Service Selector to Green
        Op->>Blue: Scale Down / Terminate
    ```

## Maintenance / Manual Recovery

1. User sets `OpenBaoCluster.Spec.Paused = true` to enter maintenance mode.
2. All reconcilers for that cluster short-circuit and stop mutating resources, allowing manual actions (e.g., manual restore from snapshot).
3. If an upgrade was in progress, it is paused but state is preserved in `Status.Upgrade`.
4. After maintenance, user sets `Paused = false` to resume normal reconciliation (including any paused upgrade).
