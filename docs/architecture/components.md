# Component Design

The OpenBao Operator uses a **Split-Controller Architecture**. Instead of a single monolithic reconciliation loop, we divide responsibilities across three specialized controllers per `OpenBaoCluster`.

## 1. Controller Hierarchy

We separate **Workload** (Pod churn), **Operations** (Upgrades/Backups), and **Status** (Updates) to prevent head-of-line blocking and status write contention.

```mermaid
graph TD
    Manager[Manager Process] -->|Starts| Workload[("fa:fa-server Workload Controller")]
    Manager -->|Starts| Admin[("fa:fa-tools AdminOps Controller")]
    Manager -->|Starts| Status[("fa:fa-pen-to-square Status Controller")]

    subgraph Roles ["Responsibilities"]
        Workload -->|Delegates to| Infra[Infra Manager]
        Workload -->|Delegates to| Cert[Cert Manager]
        
        Admin -->|Delegates to| Upgrade[Upgrade Manager]
        Admin -->|Delegates to| Backup[Backup Manager]
        
        Status -->|Aggregates| Conditions[Status Conditions]
    end
    
    classDef process fill:transparent,stroke:#9333ea,stroke-width:2px,color:#fff;
    classDef write fill:transparent,stroke:#22c55e,stroke-width:2px,color:#fff;
    classDef read fill:transparent,stroke:#60a5fa,stroke-width:2px,color:#fff;
    
    class Manager process;
    class Workload,Admin,Status write;
    class Infra,Cert,Upgrade,Backup,Conditions read;
```

---

## 2. Controllers

| Controller | Role | Why Separate? |
| :--- | :--- | :--- |
| **Workload** | Reconciles StatefulSet, Services, ConfigMaps, and Secrets. | High churn. Needs to react fast to Pod failures. |
| **AdminOps** | Handles Upgrades, Backups, and Restores. | Long-running operations. Should not block Pod recovery. |
| **Status** | Aggregates status from other controllers and writes to API. | Prevents `ResourceVersion` conflicts by serializing status updates. |

---

## 3. Internal Managers

Controllers delegate complex business logic to specialized **Internal Managers**.

```mermaid
classDiagram
    class OpenBaoClusterReconciler {
        +Reconcile()
    }

    class InfraManager {
        +SyncStatefulSet()
        +SyncService()
    }
    
    class CertManager {
        +SyncPKI()
        +RotateCerts()
    }
    
    class UpgradeManager {
        +ReconcileUpdate()
    }

    OpenBaoClusterReconciler --> InfraManager : Uses
    OpenBaoClusterReconciler --> CertManager : Uses
    OpenBaoClusterReconciler --> UpgradeManager : Uses

    style OpenBaoClusterReconciler fill:transparent,stroke:#22c55e,stroke-width:2px,color:#fff
    style InfraManager fill:transparent,stroke:#60a5fa,stroke-width:2px,color:#fff
    style CertManager fill:transparent,stroke:#60a5fa,stroke-width:2px,color:#fff
    style UpgradeManager fill:transparent,stroke:#60a5fa,stroke-width:2px,color:#fff
```

### Domain Managers

- **[Infrastructure Manager](infra-manager.md)**: The "heart" of the operator. Generates `config.hcl` and manages the `StatefulSet`.
- **[Cert Manager](cert-manager.md)**: Handles TLS interactions. Supports `OperatorManaged` (internal CA), `ACME` (LetsEncrypt), and `External` (Bring your own).
- **[Init Manager](init-manager.md)**: Auto-initializes new clusters, handling the `sys/init` call and root token encryption.
- **[Upgrade Manager](upgrade-manager.md)**: Powering both **Rolling** and **Blue/Green** upgrades. Manages the state machine for complex transitions.
- **[Backup Manager](backup-manager.md)**: Runs snapshot jobs on a Cron schedule.

### Shared Libraries

- **`internal/provisioner`**: Handles RBAC and Namespace creation for Tenants.
- **`internal/config`**: A pure-functional HCL generator that renders OpenBao configuration.
