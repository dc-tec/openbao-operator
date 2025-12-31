# Break Glass / Safe Mode

The operator can enter **Break Glass / Safe Mode** when it detects a quorum-risk failure where continuing automation could make the situation worse (for example, a failed blue/green rollback consensus repair).

When break glass is active:

- The operator halts quorum-risk automation for the affected cluster (upgrade/rollback state machines).
- The cluster reports recovery guidance in `status.breakGlass`.
- You must explicitly acknowledge the situation to let the operator resume automation.

## How to Detect Break Glass

1. Check the cluster conditions:

   ```sh
   kubectl -n <ns> describe openbaocluster <name>
   ```

2. Inspect the break glass status block:

   ```sh
   kubectl -n <ns> get openbaocluster <name> -o jsonpath='{.status.breakGlass}'
   ```

Key fields:

- `status.breakGlass.active`: `true` when safe mode is active.
- `status.breakGlass.reason`: typed reason for safe mode.
- `status.breakGlass.message`: short summary.
- `status.breakGlass.steps`: deterministic recovery steps and commands.
- `status.breakGlass.nonce`: the token required for acknowledgement.

## How to Acknowledge and Resume

1. Follow the recovery steps in `status.breakGlass.steps`.
2. Patch `spec.breakGlassAck` to match `status.breakGlass.nonce`:

   ```sh
   kubectl -n <ns> patch openbaocluster <name> --type merge \
     -p '{"spec":{"breakGlassAck":"<nonce>"}}'
   ```

If the issue is still present, the operator may re-enter break glass (with a new nonce).

## Related Runbooks

- [Recovering From No Leader / No Quorum](recovery-no-leader.md)
- [Recovering From a Sealed Cluster](recovery-sealed-cluster.md)
- [Recovering From a Failed Blue/Green Rollback](recovery-failed-rollback.md)
- [Restore After a Partial Upgrade](recovery-restore-after-upgrade.md)

