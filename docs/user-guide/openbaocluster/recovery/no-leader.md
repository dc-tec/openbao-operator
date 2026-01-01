# Recovering From No Leader / No Quorum

This runbook applies when:

- `OpenBaoCluster` reports it cannot determine a leader, or
- OpenBao pods are running but the cluster cannot form quorum.

## 1. Confirm Pod State

```sh
kubectl -n <ns> get pods -l openbao.org/cluster=<name> -o wide
kubectl -n <ns> describe pod <name>-0
```

If pods are crash-looping, address that first (configuration, PVC, networking).

## 2. Identify Leader and Peer State

The operator relies on Kubernetes service registration labels where available.

Check for `openbao-active` and related labels:

```sh
kubectl -n <ns> get pods -l openbao.org/cluster=<name> \
  -o custom-columns=NAME:.metadata.name,ACTIVE:.metadata.labels.openbao-active,SEALED:.metadata.labels.openbao-sealed,INIT:.metadata.labels.openbao-initialized
```

If you can exec into a pod, inspect Raft peers:

```sh
kubectl -n <ns> exec -it <name>-0 -- bao operator raft list-peers
```

## 3. Verify Network and DNS

Make sure pods can reach each other on the Raft ports and that the headless Service resolves:

```sh
kubectl -n <ns> get svc <name>
kubectl -n <ns> get endpoints <name>
```

If you use NetworkPolicies, confirm they allow pod-to-pod traffic within the namespace.

## 4. Decide Recovery Approach

- If a majority of peers are reachable and you can identify stale peers, you may be able to remove dead peers and restore quorum.
- If the cluster state is uncertain or you suspect split-brain, stop and follow the operator’s break glass guidance if present:
  - [Break Glass / Safe Mode](safe-mode.md)

## 5. After Recovery

Once quorum is restored, monitor:

```sh
kubectl -n <ns> get openbaocluster <name> -o yaml
```

If the operator is in break glass mode, acknowledge it to resume automation:

- [Break Glass / Safe Mode](safe-mode.md)
