---
title: Reconcile approved operator policies
description: Restore built-in OpenBao policies without granting the controller permission to approve new capabilities.
weight: 65
verifiedBy:
  - internal/adapter/config/operator_policies.go
  - internal/service/configuration/policies.go
  - test/integration/policy_reconciliation_test.go
---

Set `spec.selfInit.oidc.reconcilePolicies: true` to let the operator restore its built-in operational policies.
The feature is disabled by default and requires self-init with OIDC enabled.

```yaml
spec:
  selfInit:
    enabled: true
    oidc:
      enabled: true
      reconcilePolicies: true
```

This example shows only the relevant fields. Keep your human authentication configuration in
`spec.selfInit.requests`; operator authentication does not provide administrator access.

## Ownership and approval

The operator owns the contents of `openbao-operator`, `openbao-operator-upgrade`, and `openbao-operator-restore`.
It also owns `openbao-operator-backup` when backup is configured. Enabling this feature overwrites custom edits to
these policies. Custom JWT role names do not add other policies to this set.

A separate policy, `openbao-operator-policy-approval`, permits writes to these fixed names with exact approved
contents. OpenBao checks the request's `policy` parameter. The controller cannot update this approval policy,
change JWT roles, or repair an auth method through this grant. Keep other policies attached to the controller role
free of broader policy-write permissions; another grant can defeat this restriction.

New clusters install the initial approval during trusted self-initialization and attach it only to the controller
role. Existing clusters require administrator enrollment. Later permission changes require a new approval, including
changes from RollingUpdate to BlueGreen and policy changes introduced by an operator release.

## Enroll an existing cluster or approve a change

1. Check out the source revision that matches the intended operator release. Save the intended `OpenBaoCluster`
   manifest as `cluster.yaml`.
2. Generate the approval artifact from that checkout:

   ```sh
   devenv shell go run ./hack/tools/operator_policy_approval --cluster cluster.yaml > policy-approval.hcl
   ```

3. Review the policy paths and capabilities in the artifact. The approval covers exact text, including formatting.
   Keep it in your administrator-managed configuration repository.
4. Authenticate to the target OpenBao cluster as an administrator and apply the reviewed artifact:

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
7. Verify that `status.workload.policyRevision` is populated and `status.workload.lastError` is absent.

Replace old approval contents rather than retaining multiple accepted versions. Every retained version remains
available to the controller, including versions with permissions you intended to remove. An administrator can apply
these artifacts through an existing fleet configuration workflow; approval does not depend on Kubernetes status.

## Recovery behavior

The workload controller reapplies the bundle during reconciliation and periodic refreshes. It can recreate a deleted
operational policy while the approval policy and controller authentication still work. Writes are sequential. The
operator records `policyRevision` only after the complete bundle succeeds and clears it after a failed write.
New backup, upgrade, and restore operations wait for the intended revision. Running Jobs can finish and release
their operation locks. Kubernetes status records progress; OpenBao ACLs enforce authorization.

`PolicyReconciliationFailed` in `status.workload.lastError` identifies an unsuccessful attempt. Check the underlying
connection or permission error. An administrator must restore a missing approval policy, JWT role, or auth method.
The controller does not fall back to a root token. Adding backup after initialization can also require administrator
creation of the backup role; policy reconciliation does not create roles.

A restored snapshot can contain older approvals and auth configuration. Reapprove the intended bundle after recovery
when needed. Removing a feature does not delete its old policy or role; retire these through administrator configuration.

## Revoke policy management

Delete `openbao-operator-policy-approval` in OpenBao and remove it from the controller role. Set
`reconcilePolicies: false` to stop normal reconciliation attempts. Disabling the Kubernetes field alone does not revoke
permissions already granted in OpenBao. Revoking policy management does not revoke the controller's operational policy
or lifecycle identities; revoke those separately if required.

This feature limits OpenBao policy-write authority. The lifecycle controller remains trusted Kubernetes infrastructure
with authority to manage workloads and their identities. It does not provide containment for a compromised controller.
