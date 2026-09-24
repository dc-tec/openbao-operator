# OpenBao policy approval

Install this optional chart in an administration namespace. Set `cluster.name` and
`cluster.namespace`; the default ServiceAccount is `<release-name>-approver`.
Reference that identity through `spec.selfInit.oidc.policyApproverRef` before the
OpenBao cluster initializes.

For a later permission change, select `approval.from` and `approval.to` bundle IDs
and a digest-pinned `image`. The chart calculates hashes and request names. Review
the bundles under `bundles/` in the pinned chart revision. Existing IDs are immutable.

See [the GitOps guide](../../website/content/docs/operate/gitops-policy-approval.md)
for the normal workflow and [recovery](../../website/content/docs/operate/policy-approval-recovery.md)
for existing clusters, retries, and revocation.
