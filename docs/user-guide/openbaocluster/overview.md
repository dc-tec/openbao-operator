# OpenBaoCluster

`OpenBaoCluster` is the primary CRD that declaratively defines an OpenBao cluster on Kubernetes.

## GitOps Contract (Source of Truth)

- `spec`: Git-managed intent (the operator does not mutate `spec`).
- `status`: runtime/observed state (operator-written).
- `metadata.finalizers`: operator-written for safe deletion.

```mermaid
flowchart LR
    Git["Git (ArgoCD/Flux)"] --> Spec["OpenBaoCluster.spec"]
    Spec --> Op["Operator controllers"]
    Op --> Infra["Kubernetes resources<br/>(StatefulSet/Services/ConfigMaps/Secrets/Jobs)"]
    Op --> Status["OpenBaoCluster.status"]

    classDef write fill:#22c55e,stroke:#166534,color:#000;
    classDef read fill:#60a5fa,stroke:#1d4ed8,color:#000;

    class Spec read;
    class Status,Infra write;
```

Next:

- [Getting Started](getting-started.md)
- Configuration:
  - [Security Profiles](configuration/security-profiles.md)
  - [Self-Initialization](configuration/self-init.md)
  - [Sentinel Drift Detection](configuration/sentinel.md)
- Operations:
  - [Upgrades](operations/upgrades.md)
  - [Backups](operations/backups.md)
