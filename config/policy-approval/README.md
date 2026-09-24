# GitOps policy approval examples

The optional [approval chart](../../charts/openbao-policy-approval) packages the
Job, immutable policy bundles, ServiceAccount, and network rule.

- `values.yaml`: target and trust configuration shared across approval requests.
- `argocd.yaml`: an Application using the chart from a pinned Git revision.
- `flux.yaml`: a GitRepository and HelmRelease using the same chart.
- `examples/enrollment.yaml`: administrator enrollment for existing clusters only.

The GitOps examples install prerequisites without running a Job. Set `approval.from`
and `approval.to` to approve a later change. Replace the Git revision, target, public
CA reference, and image digest for your environment. Create the administration
namespace through your existing platform configuration.

See [Approve operator policies through GitOps](../../website/content/docs/operate/gitops-policy-approval.md).
