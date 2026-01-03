# Sentinel Architecture

The Sentinel provides a specialized high-availability loop for detecting infrastructure drift (Day 2 operations).

## Design Principles

1. **Fast Feedback**: Detects changes in seconds, unlike the standard 10h resync period.
2. **Safety First**: Cannot modify Spec or Secrets. Relies on the Operator for actual remediation.
3. **Low Impact**: Fast-path reconciliation skips expensive checks (upgrades, backups) to restore service validity execution quickly.

## Sequence Diagram

The following diagram illustrates the interaction between an External Actor, the Sentinel, and the Operator.

```mermaid
sequenceDiagram
    autonumber
    participant Ext as External Actor
    participant K8s as Kubernetes API
    participant Sen as Sentinel
    participant Op as Operator

    Note over Ext, K8s: 1. Unauthorized Change
    Ext->>K8s: Patch StatefulSet (Drift)
    K8s-->>Sen: Watch Event

    Note over Sen: 2. Filter & Debounce
    Sen->>Sen: Check Actor != Operator
    Sen->>Sen: Wait Debounce Window (2s)

    Note over Sen, Op: 3. Trigger Remediation
    Sen->>K8s: Patch Status (triggerID)
    K8s-->>Op: Watch Event (Status Change)
    
    Op->>Op: Enter Fast Path
    Op->>K8s: Revert StatefulSet
    Op->>K8s: Update Status (lastHandledTriggerID)

    Note over Op: 4. Consistency Check
    Op->>Op: Schedule Full Reconcile
```

## Security Model

Sentinel operates under a strict **Zero Trust** model enforced by Kubernetes primitives.

| Component | Policy | Effect |
| :--- | :--- | :--- |
| **RBAC** | `Role` / `RoleBinding` | Read-only access to Infra. No Secret access. |
| **VAP** | `ValidatingAdmissionPolicy` | Blocks all mutations except `status.sentinel.*`. |
| **Logic** | Code Level | Ignores own updates via `managedFields`. |

## Fast Path Logic

When the Operator observes a Sentinel trigger:

1. **Verify**: Checks `status.sentinel.triggerID`.
2. **Fast Path**:
    - Runs `InfraManager` (StatefulSet, Service, ConfigMap).
    - Runs `CertManager` (TLS validity).
    - **Skips** `UpgradeManager` (Rolling/BlueGreen logic).
    - **Skips** `BackupManager` (Snapshot logic).
3. **Acknowledge**: Updates `status.sentinel.lastHandledTriggerID`.
4. **Requeue**: Enqueues a full reconciliation (standard path) to ensure system convergence.
