# 11. Network Configuration

[Back to User Guide index](README.md)

### 11.1 API Server CIDR Fallback

In restricted multi-tenant environments, the operator may not have permissions to read Services/Endpoints in the `default` namespace, which prevents auto-detection of the Kubernetes API server CIDR for NetworkPolicy egress rules.

You can manually configure the API server CIDR as a fallback:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: restricted-cluster
spec:
  # ... other spec fields ...
  network:
    # Manual API server CIDR configuration (fallback when auto-detection fails)
    apiServerCIDR: "10.43.0.0/16"  # Service network CIDR
    # Or for self-managed clusters:
    # apiServerCIDR: "192.168.1.0/24"  # Control plane node CIDR
```

**When to Use:**
- The operator lacks permissions to read the `kubernetes` Service in the `default` namespace
- Auto-detection fails due to network restrictions
- You want to explicitly control the NetworkPolicy egress rules

**How to Determine the CIDR:**
- **Managed clusters (EKS, GKE, AKS)**: Use the service network CIDR (e.g., `10.43.0.0/16`). You can find this by checking the ClusterIP of the `kubernetes` Service: `kubectl get svc kubernetes -n default -o jsonpath='{.spec.clusterIP}'`
- **Self-managed clusters (k3d, kubeadm)**: Use the control plane node CIDR or the specific API server endpoint IP with `/32` (e.g., `192.168.1.100/32`)
