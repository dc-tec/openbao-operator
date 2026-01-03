# UpgradeManager (Rolling & Blue/Green)

!!! tip "User Guide"
    For operational instructions, see the [Upgrades User Guide](../user-guide/openbaocluster/operations/upgrades.md).

**Responsibility:** Orchestrate safe version updates while maintaining Raft consensus.

## 1. Upgrade Strategies

The Manager supports two distinct strategies, controlled by `spec.updateStrategy`.

=== "Rolling Update (Default)"

    **Goal:** Update pods one-by-one with minimal downtime.

    The Manager uses **StatefulSet Partitioning** to control the rollout.

    ```mermaid
    graph TD
        Trigger[Version Change] -->|Pause| Partition[Set Partition = Replicas]
        Partition --> Loop{Partition > 0?}
        
        Loop -- Yes --> Ident[Identify Leader]
        Ident -->|If Target is Leader| StepDown[Force Step-Down]
        Ident -->|If Target is Follower| Update[Decrement Partition]
        
        StepDown --> WaitTransfer[Wait for Leadership Transfer]
        WaitTransfer --> Update
        
        Update --> WaitReady[Wait for Pod Ready]
        WaitReady --> WaitHealth[Wait for Vault Health]
        WaitHealth --> Loop
        
        Loop -- No --> Done[Upgrade Complete]

        classDef process fill:transparent,stroke:#9333ea,stroke-width:2px,color:#fff;
        classDef write fill:transparent,stroke:#22c55e,stroke-width:2px,color:#fff;
        classDef read fill:transparent,stroke:#60a5fa,stroke-width:2px,color:#fff;
        
        class Partition,StepDown,Update write;
        class Trigger,Ident,WaitReady,WaitHealth read;
        class Loop,WaitTransfer process;
    ```

    1.  **Partitioning:** We pause Kubernetes updates by setting `partition` equal to `replicas`.
    2.  **Reverse Ordinal:** We update from highest index (e.g., 2) down to 0.
    3.  **Leader Safety:** Before updating the node that is currently the **Leader**, we send `PUT /sys/step-down` to force a leadership transfer. This prevents the cluster from crashing during the leader's restart.

=== "Blue/Green"

    **Goal:** Zero-downtime upgrades with instant rollback.

    !!! warning "Resource Usage"
        Requires **2x Storage** capacity during the transition (Blue volume + Green volume).

    This strategy spawns a parallel "Green" cluster and migrates data via Raft.

    ```mermaid
    graph TD
        Start((Start)) -->|v1 -> v2| Deploy[Deploy Green Cluster]
        
        subgraph Preparation
            Deploy -->|Wait Ready| Join[Join Raft Mesh]
            Join -.->|Non-Voters| Sync[Sync Data]
            Sync -->|Wait Index| Promote[Promote Green to Voters]
        end

        subgraph Critical_Section [Traffic Switch]
            Promote --> Switch[Update Service Selector]
            Switch -->|Traffic -> Green| Stabilization{Stable?}
            
            Stabilization -- No --> Rollback[Rollback: Point Service to Blue]
            Stabilization -- Yes --> Demote[Demote Blue to Non-Voters]
        end

        subgraph Cleanup
            Demote --> Remove[Remove Blue from Raft]
            Remove --> Delete[Delete Blue StatefulSet]
            Rollback --> RemoveGreen[Remove Green from Raft]
            RemoveGreen --> DeleteGreen[Delete Green StatefulSet]
        end

        Delete --> Done((Idle))
        DeleteGreen --> Done

        classDef process fill:transparent,stroke:#9333ea,stroke-width:2px,color:#fff;
        classDef critical fill:transparent,stroke:#dc2626,stroke-width:2px,stroke-dasharray: 5 5,color:#fff;
        classDef write fill:transparent,stroke:#22c55e,stroke-width:2px,color:#fff;
        
        class Deploy,Join,Sync,Promote,Demote,Remove,Delete,RemoveGreen,DeleteGreen process;
        class Switch,Rollback critical;
        class Start,Done,Stabilization write;
    ```

    **Key Phases:**

    | Phase | Description |
    | :--- | :--- |
    | **Deploying** | Creates the new StatefulSet. |
    | **Joining** | Adds new pods as **Non-Voters** (Read-Only). |
    | **Promoting** | Promotes new pods to **Voters** (Write-Access). |
    | **Switching** | Updates Service selector to point to Green. |
    | **Cleanup** | Removes Blue pods from the Raft peer list. |

## 2. Upgrade State Machine

### Resumability

Upgrades are designed to survive Operator restarts. All state is stored in `Status`:

- **Rolling:** Tracks `Status.Upgrade.CurrentPartition` and `CompletedPods`.
- **Blue/Green:** Tracks `Status.BlueGreen.Phase` and `JobFailureCount`.

If the Operator crashes, it reads the Status on startup and **resumes** exactly where it left off.

### Image Verification

If `spec.imageVerification.enabled` is `true`:

- **Rolling:** The main StatefulSet image is pinned to the verified digest.
- **Blue/Green:** The Green StatefulSet and all executor Jobs are pinned to the verified digest.

## 3. Reconciliation Semantics

- **Idempotency:** Re-running a phase multiple times does not cause side effects (e.g., "Join" checks if already joined).
- **Safety:** The Operator prioritizes **Availability** over Progress. If a health check fails, the upgrade pauses/retries indefinitely (Rolling) or triggers a rollback (Blue/Green).
- **OwnerReferences:** Executor jobs in Blue/Green are owned by the Cluster CR, ensuring easy cleanup.
