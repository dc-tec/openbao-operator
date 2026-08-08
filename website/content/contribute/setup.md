---
title: Set up a contributor workstation
description: Bootstrap repository-managed tools and choose a local controller development loop.
eyebrow: Contribute
weight: 1
verifiedBy:
  - mk/development.mk
  - mk/deploy.mk
  - mk/build.mk
  - go.mod
  - .github/tools/package.json
  - .hugo-version
  - .devcontainer/post-install.sh
---

Run the repository-managed bootstrap before changing code. Use a cluster loop only when the behavior depends on admission, RBAC, networking, storage, or workload lifecycle.

## Install prerequisites

The repository currently expects Go 1.26.5, Docker, `kubectl` 1.33 or newer, Helm 3, Trivy, Python 3, and access to a
Kubernetes cluster. Node.js 22 and pnpm 10.34.5 remain scoped to the repository's AST tooling; the documentation site
does not use them. The devcontainer installs Hugo 0.164.0; use the documented Nix command on a host workstation.

Kind and Tilt are optional. Kind provides a reproducible local cluster; Tilt shortens repeated in-cluster rebuilds.

{{< command label="verify" title="Bootstrap and inspect the workstation" >}}
make bootstrap
make doctor
{{< /command >}}

`make bootstrap` installs repository-managed tools and local Git hooks. Treat the hook bypass variables as deliberate one-off exceptions, not a normal workflow.

## Choose a development loop

| Loop | Use it for | Commands |
| --- | --- | --- |
| Host controller | Fast controller, renderer, and decision-logic work | `make install`, then `make run-controller` |
| In-cluster controller | Admission, RBAC, networking, and lifecycle behavior | Build, load, and deploy a development image |
| Tilt | Repeated in-cluster controller edits | `make tilt-up`, then `make tilt-down` |

{{< command label="apply" title="Run the controller on the host" >}}
make bootstrap
make doctor
make install
make run-controller
{{< /command >}}

The host loop does not reproduce webhook reachability or cluster-local NetworkPolicy behavior.

{{< command label="apply" title="Run the controller in Kind" >}}
kind create cluster --name openbao-dev
make docker-build IMG=openbao-operator:dev
kind load docker-image openbao-operator:dev --name openbao-dev
make deploy IMG=openbao-operator:dev
kubectl get pods -n openbao-operator-system
{{< /command >}}

## Establish the local baseline

{{< command label="verify" title="Run the PR-equivalent core gate" >}}
make bootstrap
make doctor
make ci-core
{{< /command >}}

Use targeted tests during development, then rerun the core gate before review. If controller boundaries change, also regenerate and verify the architecture rules.

{{< command label="verify" title="Verify architecture policy" >}}
make generate-ast-rules
make verify-arch-policy
make test-ast
make lint-ast
{{< /command >}}

Continue with [project standards]({{< relref "/contribute/standards.md" >}}) and [testing]({{< relref "/contribute/testing.md" >}}).
