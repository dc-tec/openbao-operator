---
description: Step-by-step recipe for bootstrapping a local k3d cluster, installing ArgoCD, and reconciling the OpenBao validation lanes through an ApplicationSet.
---

# ArgoCD on k3d for OpenBao Validation Lanes

This recipe bootstraps the local GitOps validation environment used in `openbao-operator-test`.

It installs and configures:

- a k3d cluster
- Gateway API and cert-manager
- Traefik, monitoring, RustFS, and `infra-bao`
- ArgoCD exposed behind the shared local edge
- an `ApplicationSet` that reconciles:
  - `openbao-dev`
  - `openbao-acme`
  - `openbao-hardened`

!!! success "Based on the project test environment"
    This recipe follows the local ArgoCD validation lane implemented in `openbao-operator-test`, including the bootstrap order and the `ApplicationSet` shape used there.

!!! note "Delivery model"
    This recipe documents the ArgoCD operating model layered onto the local k3d validation environment. Treat it as a delivery workflow that complements the local architectures rather than a separate runtime topology.

## Prerequisites

- `k3d`
- `kubectl`
- `helm`
- access to the `openbao-operator-test` repository
- a repository Secret for ArgoCD, for example the pattern in `argocd/repository.yaml`

## Inputs

Replace these values before applying the manifests:

| Placeholder | Example | Purpose |
| :--- | :--- | :--- |
| `<test-repo-path>` | `/path/to/openbao-operator-test` | Local checkout of the test repository |
| `<repo-url>` | `git@github.com:dc-tec/openbao-operator-test.git` | Git URL used by ArgoCD |
| `<target-revision>` | `main` | Git revision synced by the `ApplicationSet` |

## Step 1: Create the local cluster

From the test repository root:

```bash
cd <test-repo-path>
make test-cluster-create
```

This creates the k3d cluster with the local ingress and registry settings used by the test environment.

## Step 2: Bootstrap cluster dependencies

Install the shared cluster dependencies:

```bash
cd <test-repo-path>
make test-cluster-bootstrap
kubectl apply -k cluster
```

This gives the local cluster the same prerequisite shape used by the validated ArgoCD lane:

- Gateway API CRDs
- cert-manager
- Traefik
- monitoring
- RustFS
- `infra-bao`
- ArgoCD

## Step 3: Apply the repository Secret

Create the repository Secret in the `argocd` namespace using your own credentials.

The test repository includes an example shape in `argocd/repository.yaml`. A minimal SSH-based Secret looks like this:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: repo-openbao-operator-test
  namespace: argocd
  labels:
    argocd.argoproj.io/secret-type: repository
stringData:
  type: git
  url: "<repo-url>"
  sshPrivateKey: |
    -----BEGIN OPENSSH PRIVATE KEY-----
    ...
    -----END OPENSSH PRIVATE KEY-----
```

Apply the Secret:

```bash
kubectl apply -f argocd/repository.yaml
```

## Step 4: Apply the ApplicationSet

Apply the GitOps manifests:

```bash
kubectl apply -k argocd
```

The validated `ApplicationSet` uses:

- `CreateNamespace=true`
- `ServerSideApply=true`
- `SkipDryRunOnMissingResource=true`

Those options are intentional. Keep them unless you are also changing the bootstrap order.

## Step 5: Verify ArgoCD and the managed applications

Check the `ApplicationSet` and generated applications:

```bash
kubectl -n argocd get applicationsets
kubectl -n argocd get applications
```

The steady-state expectation is that ArgoCD creates:

- `openbao-dev`
- `openbao-acme`
- `openbao-hardened`

Then verify the underlying clusters:

```bash
kubectl get openbaocluster -A
kubectl get openbaotenant -A
```

## Step 6: Verify ArgoCD UI access

The local test environment exposes ArgoCD through the shared Traefik Gateway at:

```text
https://argocd.adfinis.test
```

If you want the initial admin password:

```bash
kubectl -n argocd get secret argocd-initial-admin-secret \
  -o jsonpath='{.data.password}' | base64 --decode && echo
```

## Common Failures

- ArgoCD cannot fetch the repo: verify the repository Secret, repo URL, and credentials.
- The applications stay `OutOfSync` because CRDs are missing: re-run the cluster bootstrap first.
- The applications fail dry-run on first sync: keep `SkipDryRunOnMissingResource=true`.
- The local hostnames do not resolve for the ACME lane: verify the CoreDNS rewrite applied by `cluster/coredns-custom.yaml`.

## See Also

- [GitOps Recipes Overview](index.md)
- [Local Recipes Overview](../local/index.md)
