---
title: Approve operator policies through GitOps
description: Bootstrap an independent approver and select reviewed policy bundles through Git.
weight: 66
verifiedBy:
  - internal/adapter/config/operator_policies.go
  - charts/openbao-policy-approval/templates/job.yaml
  - test/manifests/policyapproval/package_test.go
---

Configure an approver once, then select a reviewed policy bundle in Git when the
operator needs different permissions. The optional `openbao-policy-approval` chart
packages the Job, bundles, identity, hashes, and network rule. It runs independently
of the runtime operator and requires OpenBao 2.6 or later.

## Set up a new cluster

1. Choose an administration namespace outside the runtime operator's write
   permissions. The example uses `openbao-admin` and Helm release `example-policy`.
   Protect that namespace and its Git repository through platform RBAC.
2. Add the approver reference to the cluster before initialization. Keep your
   existing human administrator authentication configuration:

   ```yaml
   spec:
     reconcilePolicies: true
     selfInit:
       enabled: true
       oidc:
         enabled: true
         policyApproverRef:
           namespace: openbao-admin
           name: example-policy-approver
   ```

3. Deploy `charts/openbao-policy-approval` as release `example-policy` in
   `openbao-admin` through your GitOps system, using these values:

   ```yaml
   cluster:
     name: example
     namespace: bao
   tls:
     caConfigMapName: bao-ca
   ```

   `bao-ca` contains the public `ca.crt` in `openbao-admin`. Omit the TLS setting
   when the endpoint uses system trust. The ServiceAccount name defaults to
   `<release-name>-approver`; set `serviceAccount.name` to use another name.
   The chart also installs an ingress rule in the OpenBao namespace. If the
   administration namespace restricts egress, permit DNS and the target API.

Bootstrap installs the initial approval and binds the approver's role to the
referenced ServiceAccount. **No initial approval Job or custom self-init requests
are needed.** The runtime operator does not create the administrative ServiceAccount
or maintain its OpenBao role after initialization.

Use the complete Argo CD and Flux examples under `config/policy-approval/`, pinned
to a reviewed Git commit containing the chart. Existing clusters need
[one-time administrator enrollment]({{< relref "/docs/operate/policy-approval-recovery.md" >}}).

## Approve a permission change

Review the desired bundle under `charts/openbao-policy-approval/bundles/` in the
pinned chart revision. Commit a transition from the previously approved bundle to
the desired bundle. For example, before changing from RollingUpdate to BlueGreen:

```yaml
image: ghcr.io/dc-tec/openbao-operator@sha256:REPLACE_WITH_REVIEWED_IMAGE_DIGEST
approval:
  from: v1/rolling-update
  to: v1/blue-green
```

Use an image containing `/policy-approval`; build from this checkout until a release
includes it. The chart derives the policy hashes, authentication audience, role, and
Job name. Changes to the request create a new Job automatically. The Job verifies
the expected current approval, applies the new bundle with compare-and-set, and
checks the result. An already-matching approval succeeds without a write.

| Bundle | Upgrade strategy | Backup permissions |
| --- | --- | --- |
| `v1/rolling-update` | RollingUpdate | No |
| `v1/rolling-update-backup` | RollingUpdate | Yes |
| `v1/blue-green` | BlueGreen | No |
| `v1/blue-green-backup` | BlueGreen | Yes |

Bundle IDs are immutable. An operator release that changes policy contents ships a
new bundle revision and retains previous bundles for transition checks. An upgrade
that keeps the same policy contents needs no new approval. Fleet targets with the
same requirements can share the same `from` and `to` values.

Wait for successful approval before deploying the operator or cluster configuration
that needs the new permissions. Approve every affected target before upgrading a
shared operator. After rollout, verify `status.workload.policyRevision` and
`status.workload.policyReconciliation.lastError` as described in the
[policy reconciliation guide]({{< relref "/docs/operate/operator-policies.md" >}}).

## GitOps ordering

- **Argo CD:** set `gitops: argocd`. The chart applies prerequisites in Sync wave
  `-2` and runs the Job in wave `-1`. Resources in later waves of the same Application
  wait for approval. For separate Applications, wait for the approval Application's
  successful sync before promoting the operator change. Selective sync skips hooks.
- **Flux:** use the supplied HelmRelease, which waits for Job completion. Promote
  the operator change after that approval release reconciles the intended values
  successfully. A previously Ready release does not prove a pending request ran.

See the upstream [Argo CD ordering](https://argo-cd.readthedocs.io/en/stable/user-guide/sync-waves/)
and [Flux HelmRelease](https://fluxcd.io/flux/components/helm/helmreleases/) documentation.

The approver has administrative authority because it can change the operator's
allowed permissions. Keep it outside namespaces managed by the operator. Bootstrap
rejects the cluster's own namespace and the controller identity; platform RBAC must
protect it from the operator's other grants. See
[recovery and revocation]({{< relref "/docs/operate/policy-approval-recovery.md" >}})
for existing clusters, failed requests, and rollbacks.
