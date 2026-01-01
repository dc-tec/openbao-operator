# Recovering From a Failed Blue/Green Rollback

This runbook applies when a blue/green upgrade rollback cannot safely complete (for example, the rollback consensus repair Job fails).

## 1. Check Break Glass Status

If the operator detected an unsafe rollback failure, it enters break glass mode and halts further quorum-risk automation.

```sh
kubectl -n <ns> get openbaocluster <name> -o jsonpath='{.status.breakGlass}'
```

Follow the recovery steps in `status.breakGlass.steps`.

## 2. Inspect the Rollback Executor Job

The rollback logic runs in an upgrade executor Job. Inspect the Job and logs:

```sh
kubectl -n <ns> get jobs | rg <name>
kubectl -n <ns> logs job/<job-name>
```

## 3. Inspect Raft Membership

If you can exec into a pod:

```sh
kubectl -n <ns> exec -it <name>-0 -- bao operator raft list-peers
```

## 4. Resume Automation (Explicit Ack)

After manual remediation, acknowledge break glass using the nonce:

```sh
kubectl -n <ns> patch openbaocluster <name> --type merge \
  -p '{"spec":{"breakGlassAck":"<nonce>"}}'
```

The operator retries rollback automation using a new, deterministic Job attempt.

## 5. If You Need to Restore

If rollback cannot be repaired, consider restoring from a known-good snapshot:

- [Restore After a Partial Upgrade](../../openbaorestore/recovery-restore-after-upgrade.md)
