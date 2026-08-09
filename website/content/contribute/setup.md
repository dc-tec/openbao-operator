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
package set that does not match them. Shared CLI versions live in `hack/dev/tool-versions.env`; Devenv, the
devcontainer, and GitHub Actions consume that manifest. The environment supplies Docker CLI, kubectl, Helm 4,
Trivy, Python 3, Kind, and Tilt.

Docker daemon access and a Kubernetes cluster remain external runtime dependencies. Kind is available for a
reproducible local cluster; Tilt shortens repeated in-cluster rebuilds. The devcontainer remains a supported fallback
while it is migrated to the same generated environment contract.

{{< command label="verify" title="Validate and enter the development environment" >}}
devenv test
devenv tasks run operator:bootstrap
devenv tasks run operator:doctor
devenv shell
{{< /command >}}

`devenv test` validates the pinned, service-independent toolchain contract. Entering or testing the environment also
configures the repository-local Git hooks idempotently. `operator:bootstrap` installs the additional repository-managed
tools required by the core contributor workflow, while `operator:doctor` checks external services such as Docker and
Kubernetes access. Run `devenv tasks list` to inspect the supported contributor entry points. Specialized Make targets
install their debugger, live-reload, mutation, or benchmark tools only when invoked.

Outside Devenv, `make bootstrap`, `make git-hooks-install`, and `make doctor` remain explicit fallbacks. Treat the hook
bypass variables as deliberate one-off exceptions, not a normal workflow.

The default shell keeps the runtime and CI toolchain small. Activate the optional editor profile when an editor needs
Gopls, Delve, or the additional Go editor helpers supplied by Devenv:

{{< command label="develop" title="Enter the shell with Go editor tooling" >}}
devenv --profile editor shell
{{< /command >}}

## Choose a development loop

| Loop | Use it for | Commands |
| --- | --- | --- |
| Host controller | Fast controller, renderer, and decision-logic work | `make install`, then `make run-controller` |
| In-cluster controller | Admission, RBAC, networking, and lifecycle behavior | Build, load, and deploy a development image |
| Tilt | Repeated in-cluster controller edits | `make tilt-up`, then `make tilt-down` |

{{< command label="apply" title="Run the controller on the host" >}}
devenv tasks run operator:bootstrap
devenv tasks run operator:doctor
devenv shell

# Run these commands inside the shell.
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
devenv test
devenv tasks run operator:bootstrap
devenv tasks run operator:doctor
devenv tasks run operator:ci-core
{{< /command >}}

Use targeted tests during development, then rerun the core gate before review. If controller boundaries change, also regenerate and verify the architecture rules.

{{< command label="verify" title="Verify architecture policy" >}}
make generate-ast-rules
make verify-arch-policy
make test-ast
make lint-ast
{{< /command >}}

Continue with [project standards]({{< relref "/contribute/standards.md" >}}) and [testing]({{< relref "/contribute/testing.md" >}}).
