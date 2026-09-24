---
title: Recover policy approval access
description: Enroll existing clusters, retry approvals, roll back permissions, and revoke administrative access.
weight: 67
verifiedBy:
  - cmd/bao-policy-approval/client.go
  - config/policy-approval/examples/enrollment.yaml
  - charts/openbao-policy-approval/templates/job.yaml
---

Use the [GitOps approval guide]({{< relref "/docs/operate/gitops-policy-approval.md" >}})
for normal setup and permission changes.

## Enroll an existing cluster

An OpenBao administrator must apply the policy and complete JWT role definition in
`config/policy-approval/examples/enrollment.yaml`. Adjust the ServiceAccount subject
and the target audience to match the approval chart. The fixed JWT role is
`openbao-operator-policy-approver`; its audience is
`openbao-policy-approval:<cluster-namespace>:<cluster-name>`.

Enroll the controller for policy reconciliation through the
[policy reconciliation guide]({{< relref "/docs/operate/operator-policies.md" >}}).
Select the chart's initial `approval.from` bundle by comparing its exact HCL contents
with the current approval. A customized approval requires administrator review and
normalization before adopting a shipped bundle. Do not assume the current policy
matches a bundle based only on the operator version.

Adding or changing `policyApproverRef` on an initialized cluster does not change its
OpenBao role. Apply that change through administrator configuration. The operator
never repairs the approver's role, policy, or auth mount during runtime reconciliation.

## Retry a request

The helper waits for a healthy HTTPS endpoint within its two-minute deadline.
TLS verification failures stop immediately. A failed compare-and-set write is not
retried within an execution. The Job has bounded retries and can recognize a
successful write whose response was lost.

After fixing connectivity or trust, change `approval.retry` to create a new Job for
the same transition:

```yaml
approval:
  from: v1/rolling-update
  to: v1/blue-green
  retry: retry-1
```

After a precondition failure, inspect the live approval and request history. Update
`from` only after reviewing the unexpected state. Never calculate a new precondition
from live state and accept it automatically. Serialize requests for each target.

## Restore deleted access

A completed Job records a past approval; it does not monitor OpenBao. If an
administrator deleted the approval, review the target state and request recreation:

```yaml
approval:
  from: absent
  to: v1/blue-green
  retry: recovery-1
```

Restore a missing approver policy, JWT role, or auth method through administrator
access. The helper does not fall back to a root token. A restored snapshot can also
require reenrollment or a different `from` bundle.

## Roll back or revoke

Cancel obsolete requests, then submit the reverse transition using the current
bundle as `from`. Rolling back Helm resources does not undo an approval already
written to OpenBao. Replacing an approval can temporarily block policy repair until
the matching operator configuration is deployed. Retire old grants through an
administrator; do not retain multiple accepted permission sets indefinitely.

Pruning the chart does not revoke OpenBao permissions. Follow the policy
reconciliation guide to revoke runtime policy management, and remove the approver
role and policy separately when retiring this workflow.

The chart mounts only public CA material and a projected ServiceAccount token. Do
not mount a server private key or grant it Kubernetes API permissions. Removing the
approver's role does not revoke already-issued OpenBao tokens immediately; their
maximum lifetime is five minutes. Separating the approval identity does not contain
an operator that can modify OpenBao server workloads.
