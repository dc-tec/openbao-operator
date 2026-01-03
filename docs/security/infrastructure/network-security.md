# Network Security

!!! abstract "Core Concept"
    The Operator enforces a **Default Deny** network posture for every OpenBao cluster. This means the OpenBao pods start in total isolation, and the Operator explicitly whitelists only the traffic required for clustering, monitoring, and management.

## Network Perimeter

The following diagram illustrates the trusted communication paths allowed through the NetworkPolicy firewall:

```mermaid
flowchart TB
    %% External Actors
    Operator["Operator Pod"]
    K8sAPI["Kubernetes API"]
    DNS["CoreDNS"]
    Peer["Raft Peers"]

    %% The OpenBao Cluster
    subgraph Cluster ["OpenBao Perimeter (Default Deny)"]
        Yield["Active Node"]
    end

    %% Ingress Rules
    Operator --"Ingress (8200)"--> Yield
    Peer --"Ingress (8200/8201)"--> Yield

    %% Egress Rules
    Yield --"Egress (443)"--> K8sAPI
    Yield --"Egress (53)"--> DNS
    Yield --"Egress (8201)"--> Peer

    %% Styling
    classDef external fill:transparent,stroke:#60a5fa,stroke-width:2px,color:#fff;
    classDef secure fill:transparent,stroke:#22c55e,stroke-width:2px,color:#fff;
    classDef blocked fill:transparent,stroke:#dc2626,stroke-width:2px,color:#fff;

    class Operator,K8sAPI,DNS,Peer external;
    class Yield secure;
```

## Traffic Rules

=== ":material-login: Ingress (Incoming)"

    By default, **all** incoming traffic is blocked. The following exceptions are made to allow the cluster to function:

    | Source | Port | Reason |
    | :--- | :--- | :--- |
    | **Operator** | `8200` (HTTP) | Required for liveness probes, initialization checks, and status updates. |
    | **Raft Peers** | `8201` (TCP) | Required for Raft consensus and replication between pods in the StatefulSet. |
    | **Gateway** | `8200` (HTTP) | (Optional) Allowed only if `spec.gateway` is enabled, permitting traffic from the Gateway API namespace. |
    | **Kube System** | Any | Required for some CNI/DNS health checks from the system namespace. |

    !!! warning "Custom Ingress"
        You can allow additional traffic (e.g., from your application namespaces) via `spec.network.ingressRules`. These rules are **additive** and cannot disable the core operator rules.

=== ":material-logout: Egress (Outgoing)"

    Egress is strictly limited to prevent data exfiltration and restrict the blast radius of a compromised path:

    | Destination | Port | Reason |
    | :--- | :--- | :--- |
    | **Kubernetes API** | `443` | Required for **Service Registration** (updating Pod labels) and **Peer Discovery**. |
    | **CoreDNS** | `53` (UDP/TCP) | Required for resolving external services and peer addresses. |
    | **Raft Peers** | `8201` (TCP) | Required for replication traffic. |

    !!! note "Backup Jobs"
        Backup and Restore jobs are **excluded** from this restrictive policy. They run as separate pods that need broad access to reach external object storage (S3, GCS, Azure).

## Sentinel Protection

To prevent the Sentinel sidecar (which watches for config drift) from overwhelming the Operator or the Kubernetes API, a strict **Rate Limiting** logic is enforced:

```mermaid
stateDiagram-v2
    [*] --> CheckCount
    
    CheckCount --> FastPath: Count < 5
    CheckCount --> ForceFull: Count >= 5
    
    FastPath --> CheckTime
    CheckTime --> Reconcile: < 5 mins
    CheckTime --> ForceFull: > 5 mins

    Reconcile --> [*]: Quick Drift Fix
    ForceFull --> [*]: Full Reconcile (Backup/Upgrade)

    classDef logic fill:transparent,stroke:#9333ea,stroke-width:2px,color:#fff;
    class CheckCount,FastPath,CheckTime,Reconcile,ForceFull logic;
```

- **Fast Path:** Simple drift correction (e.g., fixing a label).
- **Force Full:** Limits "fast checks" to prevents loops. After 5 fast corrections or 5 minutes, a full reconciliation (checking backups, upgrades, statefulsets) is forced to ensure overall system consistency.

## See Also

- [:material-network-outline: Network Configuration](../../user-guide/openbaocluster/configuration/network.md)
- [:material-policy: Admission Policies](admission-policies.md)
