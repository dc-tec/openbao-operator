# Cluster Maintenance

This guide covers maintenance operations for OpenBao clusters, including node drains, voluntary disruptions, and cluster scaling.

## Pod Disruption Budgets

The Operator automatically creates a `PodDisruptionBudget` (PDB) for each cluster with 2 or more replicas. This protects against accidental quorum loss during voluntary disruptions.

### Default Behavior

| Replicas | PDB Created | Max Unavailable | Notes |
|----------|-------------|-----------------|-------|
| 1 | No | N/A | Single-replica clusters have no redundancy |
| 3 | Yes | 1 | Ensures quorum (2/3) is always maintained |
| 5 | Yes | 1 | Conservative setting; 4 pods remain available |

The PDB uses `maxUnavailable: 1`, meaning Kubernetes will block eviction requests that would take more than one pod offline simultaneously.

### During Node Drains

When you drain a node hosting OpenBao pods:

1. **Kubernetes checks the PDB** before evicting any pods.
2. **If eviction would violate the PDB**, the drain blocks and waits.
3. **Pods are evicted one at a time**, allowing the cluster to maintain quorum.

```sh
# Safe: Node drain respects PDB
kubectl drain node-1 --ignore-daemonsets --delete-emptydir-data

# Check if PDB is blocking eviction
kubectl get pdb -n <namespace>
```

!!! warning "Drain Timeouts"
    If multiple OpenBao pods are scheduled on the same node, draining that node
    may take longer as pods are evicted sequentially. Consider using pod
    anti-affinity to spread pods across nodes.

### Limitations

The PDB only protects against **voluntary disruptions**:

- Node drains (`kubectl drain`)
- Cluster autoscaler scale-down
- Pod eviction API calls

It does **not** protect against:

- Node failures/crashes
- Pod OOM kills
- Node kernel panics

For these scenarios, rely on Raft's built-in quorum-based replication.

## Scaling

### Scaling Up

Increase replicas to add capacity or improve fault tolerance:

```yaml
spec:
  replicas: 5  # Was 3
```

The Operator will:

1. Create new pods via the StatefulSet
2. Wait for each pod to join the Raft cluster
3. Update the PDB to match the new replica count

### Scaling Down

Reduce replicas to save resources:

```yaml
spec:
  replicas: 3  # Was 5
```

!!! danger "Minimum Replicas"
    Never scale below **3 replicas** in production. Scaling to 1 replica means
    any pod failure results in complete unavailability.

The Operator will:

1. Gracefully remove Raft voters starting from the highest ordinal
2. Wait for Raft configuration to update
3. Delete the excess pods
4. Update the PDB

## Leader Step-Down

When maintenance requires the active leader to be evicted, the Operator automatically triggers a graceful step-down:

1. **Pre-eviction hook** detects the leader is being terminated
2. **Step-down request** is sent to `/sys/step-down`
3. **New leader election** completes before the old leader terminates
4. **Eviction proceeds** without service interruption

This behavior is automatic and requires no manual intervention.

## Maintenance Windows

For planned maintenance, consider:

1. **Pause reconciliation** during complex operations:
   ```yaml
   spec:
     paused: true
   ```

2. **Enable maintenance mode** when your cluster enforces managed-resource mutation locks:
   ```yaml
   spec:
     maintenance:
       enabled: true
   ```

   When enabled, the operator annotates managed Pods/StatefulSet with `openbao.org/maintenance=true`
   to support controlled deletes/restarts under strict admission policies.

3. **Trigger a rolling restart** (for example, after rotating external dependencies):
   ```yaml
   spec:
     maintenance:
       restartAt: "2026-01-19T00:00:00Z"
   ```

4. **Monitor cluster health** before and after:
   ```sh
   kubectl get openbaocluster <name> -o jsonpath='{.status.phase}'
   kubectl get pods -l openbao.org/cluster=<name>
   ```

5. **Check Raft peer status** if needed:
   ```sh
   kubectl exec -it <pod-name> -- bao operator raft list-peers
   ```
