# Day 2: Operations & Upgrades

Day 2 operations cover the ongoing management of the cluster, including version upgrades and maintenance.

!!! tip "User Guide"
    See the [Upgrade Guide](../../user-guide/openbaocluster/operations/upgrades.md) for detailed upgrade strategies (Rolling vs Blue/Green).

## Cluster Operations / Upgrades

=== "Rolling Update (Default)"

    1. User configures upgrade executor:
       - Set `spec.upgrade.image` (optional, can be inferred)
       - Set `spec.upgrade.jwtAuthRole` (optional, inferred from `spec.selfInit.oidc.enabled`) or configure manual role
    2. User updates `spec.version` and/or `spec.image` (strategy is configured via `spec.upgrade.strategy`).
    3. Upgrade Manager (adminops controller) detects version drift and performs pre-upgrade validation:
       - Validates semantic versioning (blocks downgrades by default).
       - Verifies all pods are Ready and quorum is healthy.
       - Optionally triggers a pre-upgrade backup if `spec.upgrade.preUpgradeSnapshot` is enabled.
    4. Upgrade Manager orchestrates Raft-aware rolling updates:
       - Locks StatefulSet updates using partitioning.
       - Iterates pods in reverse ordinal order.
       - Runs an upgrade Job to perform leader step-down before updating the leader pod.
       - Waits for pod Ready, OpenBao health, and Raft sync after each update.
    5. Upgrade progress is persisted in `status.upgrade` (rolling) or `status.blueGreen` (blue/green), allowing resumption after Operator restart.
    6. On completion, `status.currentVersion` is updated and `status.upgrade` is cleared (rolling), or `status.blueGreen.phase` returns to `Idle` (blue/green).

    !!! note "Upgrade Policy"
        Upgrades are designed to be safe and resumable. Downgrades are blocked by default. Rolling upgrades halt on failure and require manual intervention; Blue/Green can perform automatic rollback when `spec.upgrade.blueGreen.autoRollback.enabled=true`. Root tokens are not used for upgrade operations.

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

    Blue/Green upgrades provide zero-downtime updates by creating a parallel "Green" standby cluster and advancing it through explicit consensus phases.

    1.  **Drift Detection:** User updates `OpenBaoCluster` spec with a new version or image, using the Blue/Green strategy.
    2.  **Green Creation:** The operator creates a new "Green" StatefulSet with the new version.
    3.  **Join as Non-Voters:** Green pods start and join the existing "Blue" Raft cluster as non-voters.
    4.  **Sync and Promote:** The operator waits for Green replication to converge, then promotes Green pods to voters.
    5.  **Demote Blue and Verify Leader:** The operator demotes Blue voters, forces leadership transfer when needed, and waits until a Green leader is observed.
    6.  **Cutover During Cleanup:** The operator switches the Service selector to Green, removes Blue peers, and deletes the Blue StatefulSet. Rollback remains possible until irreversible cleanup completes.

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
        Green->>Blue: Join Raft Cluster (Non-Voters)
        Op->>Green: Wait for Sync
        Op->>Green: Promote to Voters
        Op->>Blue: Demote Blue Voters / Step Down Leader
        Op->>Green: Verify Green Leader
        Op->>K: Switch Service Selector to Green
        Op->>Blue: Remove Peers / Delete Blue StatefulSet
    ```

## Maintenance / Manual Recovery

There are two related (but distinct) mechanisms:

1. **Pause reconciliation** (`spec.paused=true`): stops all controllers for the cluster from mutating resources.
   This is intended for manual intervention or recovery workflows.
2. **Maintenance annotation mode** (`spec.maintenance.enabled=true`): keeps reconciliation running, but annotates
   managed resources with `openbao.org/maintenance=true` so admission policies can allow controlled deletes/restarts.
   The operator also uses this gate for disruptive-but-automatable operations (for example, completing filesystem
   expansion when a PVC reports `FileSystemResizePending` after increasing `spec.storage.size`).

For manual recovery:

1. User sets `spec.paused=true`.
2. Reconcilers short-circuit and stop mutating resources, allowing manual actions (e.g., manual restore from snapshot).
3. If an upgrade was in progress, it is paused but state is preserved in `status.upgrade`.
4. After maintenance, user sets `spec.paused=false` to resume normal reconciliation (including any paused upgrade).

## Official OpenBao Documentation

- [Upgrade Guide](https://openbao.org/docs/upgrading/)
- [Operator Step-Down Command](https://openbao.org/docs/commands/operator/step-down/)
- [Operator Raft Command](https://openbao.org/docs/commands/operator/raft/)
- [Recovery Mode Concepts](https://openbao.org/docs/concepts/recovery-mode/)
