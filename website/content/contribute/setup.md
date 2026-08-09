---
title: Set up a contributor workstation
description: Bootstrap repository-managed tools and choose a local controller development loop.
eyebrow: Contribute
weight: 1
verifiedBy:
  - devenv.nix
  - devenv.yaml
  - devenv.lock
  - hack/dev/verify-devenv.sh
  - hack/dev/install-ast-grep.sh
  - mk/dependencies.mk
  - mk/development.mk
  - mk/deploy.mk
  - mk/build.mk
  - go.mod
  - .hugo-version
  - .devcontainer/post-install.sh
---

Use the pinned development environment and run the repository-managed bootstrap before changing code. Use a cluster
loop only when the behavior depends on admission, RBAC, networking, storage, or workload lifecycle.

## Install prerequisites

Install [Nix](https://nixos.org/download/) and [devenv](https://devenv.sh/getting-started/), then let `devenv.lock`
select the package set. The environment reads the repository's existing Go and Hugo declarations and rejects a
package set that does not match them. It also supplies Docker CLI, `kubectl` 1.33 or newer, Helm 3, Trivy, Python 3,
Kind, and Tilt.

Docker daemon access and a Kubernetes cluster remain external runtime dependencies. Kind is available for a
reproducible local cluster; Tilt shortens repeated in-cluster rebuilds. The devcontainer remains a supported fallback
while it is migrated to the same generated environment contract.

{{< command label="verify" title="Validate and enter the development environment" >}}
devenv test
devenv shell

# Run these commands inside the shell.
make bootstrap
make doctor
{{< /command >}}

`devenv test` validates the pinned, service-independent toolchain contract. `make bootstrap` then installs the tools
required by the core contributor workflow plus local Git hooks. Specialized targets install their debugger,
live-reload, mutation, or benchmark tools only when invoked. Treat the hook bypass variables as deliberate one-off
exceptions, not a normal workflow.

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
devenv shell

# Run these commands inside the shell.
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
