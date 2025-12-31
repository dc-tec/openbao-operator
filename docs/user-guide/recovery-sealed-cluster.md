# Recovering From a Sealed Cluster

This runbook applies when OpenBao pods are running but remain **sealed**.

## 1. Confirm Seal State

```sh
kubectl -n <ns> get pods -l openbao.org/cluster=<name> \
  -o custom-columns=NAME:.metadata.name,SEALED:.metadata.labels.openbao-sealed,INIT:.metadata.labels.openbao-initialized
```

If pods are `sealed=true`, proceed based on the configured unseal mode.

## 2. Static Unseal (Operator-Managed)

If you use the default static seal, OpenBao reads the unseal key from the unseal Secret mounted into the pod.

Verify the Secret exists:

```sh
kubectl -n <ns> get secret <name>-unseal-key
```

If you recently restored from a snapshot into a **different** cluster identity, static unseal may require updating the unseal Secret to match the snapshot’s seal key material.

## 3. External KMS / Transit Unseal

If you use external unseal:

- Verify the credentials Secret (if applicable) exists in the cluster namespace.
- Verify NetworkPolicies allow egress to the KMS / transit endpoint.
- Verify the KMS key configuration is correct.

## 4. Break Glass

If the operator has entered break glass mode, follow the steps in `status.breakGlass`:

- [Break Glass / Safe Mode](recovery-safe-mode.md)

