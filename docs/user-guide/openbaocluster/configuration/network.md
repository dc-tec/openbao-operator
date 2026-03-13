# Network Configuration

OpenBao Operator automatically configures Kubernetes NetworkPolicies to secure your cluster by default using a "Deny All" + "Allow Essential" strategy.

## Default Topology

The following diagram illustrates the allowed traffic flows.

```mermaid
flowchart TB
    subgraph External["External World"]
        GW[Gateway / Ingress]
        Client[Clients]
    end

    subgraph Cluster["Kubernetes Cluster"]
        API[K8s API]
        DNS[CoreDNS]
        
        subgraph OperatorNS["Operator Namespace"]
            Op[Operator]
        end

        subgraph TenantNS["Tenant Namespace"]
            Bao[OpenBao Pods]
        end
    end

    %% Ingress Flows
    GW & Op -->|"HTTPS (8200)"| Bao
    Client -.->|"HTTPS (443)"| GW
    
    %% Internal Flows
    Bao <-->|"Raft (8201)"| Bao

    %% Egress Flows
    Bao -->|"DNS (53)"| DNS
    Bao -->|"K8s (443/6443)"| API

    classDef write fill:transparent,stroke:#22c55e,stroke-width:2px,color:#fff;
    classDef read fill:transparent,stroke:#60a5fa,stroke-width:2px,color:#fff;
    classDef process fill:transparent,stroke:#9333ea,stroke-width:2px,color:#fff;

    class Bao write;
    class Op,GW process;
    class Client,API,DNS read;
```

### Default Rules Reference

The Operator ensures these rules always exist to keep the cluster functional.

| Direction | Source / Dest | Port | Purpose |
| :--- | :--- | :--- | :--- |
| **Ingress** | **Operator** | `8200` | Health checks, Initialization, Unsealing. |
| **Ingress** | **Self** | `8201` | Raft consensus replication between peers. |
| **Ingress** | **Gateway/Ingress** | `8200` | External traffic (if Ingress/Gateway is enabled). |
| **Ingress** | **Kube-System** | Any | Readiness probes (often from kubelet/monitoring). |
| **Egress** | **Kube-DNS** | `53` | Service discovery. |
| **Egress** | **K8s API** | `443` | Kubernetes Auth Method validation. |
| **Egress** | **Self** | `8201` | Raft consensus replication. |

## DNS Configuration

By default, the NetworkPolicy allows egress to DNS services in the `kube-system` namespace. If your cluster uses a different namespace for DNS (e.g., `openshift-dns` on OpenShift), you must explicitly configure it.

```yaml
spec:
  network:
    dnsNamespace: "openshift-dns" # (1)!
```

1.  Defaults to `kube-system` if not specified.

!!! warning "DNS Resolution Failure"
    If `dnsNamespace` does not match your cluster's actual DNS namespace, OpenBao pods will fail to resolve addresses (including Cloud KMS or Storage endpoints), leading to crash loops.

If your cluster resolves DNS through node-local or host-networked caches instead of pod-backed DNS Services, also configure `dnsEndpointIPs`. The operator adds direct TCP/UDP port `53` egress rules for those resolver IPs in both the main workload and backup/restore Job NetworkPolicies.

```yaml
spec:
  network:
    dnsNamespace: "kube-system"
    dnsEndpointIPs:
      - "169.254.20.10" # Example: NodeLocal DNSCache
```

Use `dnsEndpointIPs` when:

- DNS traffic is enforced against resolver IPs instead of pod IPs.
- The resolver runs on the node, host network, or another topology outside the DNS namespace pod model.
- `dnsNamespace` is correct but OpenBao still cannot resolve names under strict NetworkPolicy enforcement.

## Custom Rules (Advanced)

You can append **additional** rules to the default policy to allow integrations like backups or monitoring.

=== "Trusted Ingress Peers"
    Allow a user-managed ingress controller or passthrough proxy to reach OpenBao on port `8200` without writing a full raw ingress rule.

    ```yaml
    spec:
      network:
        trustedIngressPeers:
          # Example: Allow all pods in the Traefik namespace
          - namespaceSelector:
              matchLabels:
                kubernetes.io/metadata.name: traefik

          # Example: Allow only specific ingress-controller pods
          - namespaceSelector:
              matchLabels:
                kubernetes.io/metadata.name: ingress-system
            podSelector:
              matchLabels:
                app.kubernetes.io/name: traefik
    ```

    Use this when traffic reaches OpenBao through a user-managed TCP proxy, passthrough ingress controller, or Gateway data plane that the Operator does not manage directly.

=== "Egress Rules"
    Allow OpenBao to connect to external services (e.g., Transit Vault, S3, Databases).

    ```yaml
    spec:
      network:
        egressRules:
          # Example: Allow access to Transit Vault in operator namespace
          - to:
              - namespaceSelector:
                  matchLabels:
                    kubernetes.io/metadata.name: openbao-operator-system
            ports:
              - protocol: TCP
                port: 8200
          
          # Example: Allow access to S3 CIDR for Backups
          - to:
              - ipBlock:
                  cidr: 192.168.100.0/24
            ports:
              - protocol: TCP
                port: 443
    ```

=== "Ingress Rules"
    Add raw ingress rules when you need full control over ports and peers beyond the common ingress-controller case.

    ```yaml
    spec:
      network:
        ingressRules:
          # Example: Allow Prometheus from monitoring namespace
          - from:
              - namespaceSelector:
                  matchLabels:
                    kubernetes.io/metadata.name: monitoring
            ports:
              - protocol: TCP
                port: 8200
    ```

## Advanced Routing

Configuring how OpenBao reaches the Kubernetes API server for Auth Method validation.

=== "Auto-Detection (Default)"
    The Operator allow-lists the in-cluster Kubernetes service VIP (`KUBERNETES_SERVICE_HOST`) as a single-host CIDR (`/32` for IPv4, `/128` for IPv6) on port `443`.

    This does not require cross-namespace RBAC reads.

    The status condition `APIServerNetworkReady` reports this path as `Unknown` with reason `APIServerEndpointIPsRecommended`.
    That means the common service-VIP path is configured, but some CNIs still require explicit endpoint IPs.

=== "Manual CIDR"
    **Use Case:** Override the detected VIP allow-list (for example, if you want to allow a larger CIDR).

    ```yaml
    spec:
      network:
        # Prefer single-host CIDRs when possible (least privilege).
        # Example (k3s): "10.43.0.1/32"
        apiServerCIDR: "10.43.0.1/32"
    ```

=== "Endpoint IPs"
    **Use Case:** CNIs / NetworkPolicy implementations that enforce egress on post-DNAT traffic.

    In these environments, allowing only the Service VIP (port `443`) may not be sufficient because traffic is evaluated against the backing API server endpoint IP (commonly port `6443`).

    The Operator does not auto-detect these endpoint IPs because that would require broader cluster permissions (list/watch).

    ```yaml
    spec:
      network:
        apiServerEndpointIPs:
          - "192.168.166.2" # The IP of the API Server container/node
    ```

    When this field is set and valid, `APIServerNetworkReady=True`.

## Troubleshooting Signal

Use `APIServerNetworkReady` to interpret Kubernetes API egress behavior:

- `False` with reason `APIServerNetworkConfigurationInvalid`:
  The operator could not build a safe Kubernetes API egress allow-list.
- `Unknown` with reason `APIServerEndpointIPsRecommended`:
  The service VIP path is configured, but post-DNAT endpoint IPs may still be required in your environment.
- `True` with reason `APIServerNetworkReady`:
  The operator has a concrete service-VIP plus endpoint-IP contract for Kubernetes API egress.
