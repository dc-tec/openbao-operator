---
title: Reconcile approved operator policies
description: Restore built-in OpenBao policies without granting the controller permission to approve new capabilities.
weight: 65
verifiedBy:
  - internal/adapter/config/operator_policies.go
  - internal/service/configuration/policies.go
  - test/integration/policy_reconciliation_test.go
---

Set `spec.reconcilePolicies: true` to let the operator restore its built-in operational policies.
The feature is disabled by default. It requires an administrator-enrolled controller JWT role and exact-content approval.
It also works on clusters initialized without self-init.

```yaml
spec:
  reconcilePolicies: true
```

On new clusters, enable `selfInit.enabled` and `selfInit.oidc.enabled` to bootstrap the controller role and initial
approval. Keep your human administrator authentication configuration; operator authentication does not provide administrator access.

## Ownership and approval

The operator owns the contents of `openbao-operator`, `openbao-operator-upgrade`, and `openbao-operator-restore`.
It also owns `openbao-operator-backup` when backup is configured. Enabling this feature overwrites custom edits to
these policies. Custom JWT role names do not add other policies to this set.

A separate policy, `openbao-operator-policy-approval`, permits reading these fixed names and writing exact approved
contents. OpenBao checks the write request's `policy` parameter. The controller cannot update this approval policy,
change JWT roles, or repair an auth method through this grant. Keep other policies attached to the controller role
free of broader policy-write permissions; another grant can defeat this restriction.

New clusters with OIDC bootstrap install the initial approval during trusted self-initialization and attach it only to the controller
role. Existing clusters require administrator enrollment. Later permission changes require a new approval, including
changes from RollingUpdate to BlueGreen and policy changes introduced by an operator release.

## Enroll an existing cluster or approve a change

For a manually initialized cluster, first configure the `jwt-operator` auth mount to trust the operator installation's
projected Kubernetes ServiceAccount token, and enroll its `openbao-operator` role with the correct subject and audience.
Policy reconciliation does not create or repair this authentication configuration.

1. Check out the source revision that matches the intended operator release. Save the intended `OpenBaoCluster`
   manifest as `cluster.yaml`.
2. Generate the approval artifact from that checkout:

   ```sh
   devenv shell go run ./hack/tools/operator_policy_approval --cluster cluster.yaml > policy-approval.hcl
   ```

3. Review the policy paths and capabilities in the artifact. The approval covers exact text, including formatting.
   Keep it in your administrator-managed configuration repository.
4. Authenticate to the target OpenBao cluster as an administrator and apply the reviewed artifact.
   If the approval policy already requires CAS, follow [Update with compare-and-set](#update-with-compare-and-set) instead.

   ```sh
   bao policy write openbao-operator-policy-approval policy-approval.hcl
   ```

5. For initial enrollment, add `openbao-operator-policy-approval` to `token_policies` in the complete
   administrator-managed definition of `auth/jwt-operator/role/openbao-operator`. Preserve its existing subject,
   audience, issuer-related claims, and token settings. Apply that complete definition. A partial role write can
   reset omitted settings. Do not attach the approval policy to backup, upgrade, or restore roles.
6. Enable `reconcilePolicies` and deploy the intended operator configuration. With standard JWT authentication,
   existing cached tokens might not include the new policy until they expire. Restart the controller to obtain new
   tokens if needed. Inline JWT authentication uses the updated role on its next request.
7. Verify that `status.workload.policyRevision` is populated and `status.workload.policyReconciliation.lastError` is absent.

Replace old approval contents rather than retaining multiple accepted versions. Every retained version remains
available to the controller, including versions with permissions you intended to remove. An administrator can apply
these artifacts through an existing fleet configuration workflow; approval does not depend on Kubernetes status.

Apply permission changes before deploying the operator release or cluster configuration that needs them. An operator
upgrade with unchanged policy contents needs no new approval. For a shared operator, approve every affected cluster
before upgrading it.

Wait for an active upgrade to finish before changing the approved strategy. During an unfinished upgrade, runtime
reconciliation retains the accepted strategy's policy contents, including when its operation lock must be recovered.
It reconciles the requested strategy's policy after the upgrade finishes. Repairing a deleted policy during that
interval still requires approval for the running strategy.

### Update with compare-and-set

On OpenBao 2.6 or later, administrators can use compare-and-set (CAS) to avoid overwriting concurrent approval changes.
Read the current version, then supply it with the reviewed policy. For a missing approval policy, use `cas=-1` instead.

```sh
bao read -field=version sys/policies/acl/openbao-operator-policy-approval
bao write sys/policies/acl/openbao-operator-policy-approval \
  policy=@policy-approval.hcl cas=REPLACE_WITH_VERSION cas_required=true
```

Setting `cas_required=true` requires CAS on later updates. If the CAS check fails, read and review the current approval
before retrying. CAS protects writes; it does not guarantee a fresh read from a standby.

## Recovery behavior

The workload controller reads each policy during reconciliation and periodic refreshes, approximately once per minute
by default. It writes only missing or changed contents. A failed policy does not stop checks and repairs for the others,
infrastructure reconciliation, or Autopilot configuration.

`status.workload.policyReconciliation.revisions` records each policy's last verified digest. A missing or changed policy
loses its entry until repaired. A failed read preserves the previous observation. OpenBao authorizes every operation;
these observations do not grant access. New backup and upgrade operations check only their own policy. Running operations
can finish and release their locks. Restore never waits for policy reconciliation; its configured OpenBao credentials must
still authorize the restore.

`status.workload.policyRevision` retains the last completely verified bundle. It is informational and does not gate operations.
`status.workload.policyReconciliation.lastError` reports policy failures separately from other workload errors.
Forbidden requests (403) and missing auth or write endpoints (404) retry after five minutes. Other failures retry after
30 seconds. Unrelated reconciles respect `retryAfter`; a desired bundle change triggers a new attempt. A missing ACL policy
on read is repaired immediately when approved.

An administrator must restore a missing approval policy, JWT role, or auth method. The controller does not fall back to a
root token. Adding backup after initialization can also require administrator creation of the backup role; policy
reconciliation does not create roles.

A restored snapshot can contain older approvals and auth configuration. Reapprove the intended bundle after recovery
when needed. Removing a feature does not delete its old policy or role; retire these through administrator configuration.

## Revoke policy management

Delete `openbao-operator-policy-approval` in OpenBao and remove it from the controller role. Set
`spec.reconcilePolicies: false` to stop normal reconciliation attempts. Disabling the Kubernetes field alone does not revoke
permissions already granted in OpenBao. Revoking policy management does not revoke the controller's operational policy
or lifecycle identities; revoke those separately if required.

This feature limits OpenBao policy-write authority. The lifecycle controller remains trusted Kubernetes infrastructure
with authority to manage workloads and their identities. It does not provide containment for a compromised controller.
