---
title: Setting Up Your Environment
description: Contributor environment setup for OpenBao Operator, including prerequisites, recommended development loops, local verification, and troubleshooting.
pageType: task
journey: contribute
---

<PageHeader
  title="Get a workstation to the point where you can build, run, test, and debug the operator without fighting the toolchain."
  lede="Start by bootstrapping the repository-managed tools, then choose the smallest development loop that matches your task. Most contributor work does not need a full cluster deployment on the first edit, but webhooks, RBAC, networking, and lifecycle behavior eventually do."
/>

## Prerequisites

Required tools:

- Go `1.26.2+`
- Docker `28.3.3+`
- `kubectl` `1.33+`
- Helm `3+`
- Trivy
- Python `3`
- Node.js `20+` with `npm`
- a Kubernetes cluster such as Kind, Minikube, or a cloud cluster

Optional but recommended:

- Kind for local E2E work
- k9s for cluster inspection
- Tilt for the fast in-cluster inner loop

<CommandBlock
  language="bash"
  label="verify"
  title="Bootstrap the repository-managed toolchain"
  code={`make bootstrap
make doctor`}
>
  Run this first on a new machine, inside a fresh devcontainer, or after toolchain changes. `make doctor` is the quickest way to see what is still missing locally.
</CommandBlock>

<DecisionTable
  title="Choose a development loop"
  columns={["Use this loop", "Best for", "Main command path"]}
  rows={[
    {
      cells: [
        "Local controller on your host",
        "Fast logic changes, renderer work, and controller behavior that does not depend on webhook reachability or cluster-local network policy.",
        "`make install` then `make run-controller`.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "In-cluster integration loop",
        "RBAC, webhook, lifecycle, and deployment behavior where the controller must run as a Pod inside the cluster.",
        "Build image, load it into Kind, then `make deploy IMG=...`.",
      ],
    },
    {
      cells: [
        "Tilt fast cluster loop",
        "Repeated controller changes against a local cluster when you want rebuilds, deploys, logs, and helper actions in one UI.",
        "`make tilt-up` and `make tilt-down`.",
      ],
    },
  ]}
/>

<CommandBlock
  language="bash"
  label="apply"
  title="Fast local controller loop"
  code={`git clone https://github.com/dc-tec/openbao-operator.git
cd openbao-operator

make bootstrap
make doctor
make install
make run-controller`}
>
  This is the default contributor loop. It is fast, but it does not fully represent webhook reachability or cluster-local network behavior.
</CommandBlock>

<Callout type="warning" title="Local loop limitations">

When the controller runs on your host, validating webhooks usually need additional tunneling or a different deployment path. NetworkPolicies are also not realistically tested from this loop.

</Callout>

<CommandBlock
  language="bash"
  label="apply"
  title="In-cluster integration loop"
  code={`kind create cluster --name openbao-dev

make bootstrap
make doctor
make docker-build IMG=openbao-operator:dev
kind load docker-image openbao-operator:dev --name openbao-dev
make deploy IMG=openbao-operator:dev

kubectl get pods -n openbao-operator-system`}
>
  Use this when the feature depends on cluster-local execution, admission behavior, or realistic operator RBAC.
</CommandBlock>

<CommandBlock
  language="bash"
  label="apply"
  title="Tilt fast cluster loop"
  code={`make bootstrap
make doctor
make tilt-up

# optional helper image override
tilt up -- --operator-version=edge

make tilt-down`}
>
  Tilt is the best inner loop when you expect several controller edits against the same local cluster and want logs, deploy state, and helper actions in one place.
</CommandBlock>

<DecisionTable
  title="High-value local commands"
  kind="reference"
  columns={["Area", "Command", "What it gives you"]}
  rows={[
    {
      cells: [
        "Core verification",
        "`make ci-core`",
        "The default local PR-equivalent gate except E2E.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Unit and integration",
        "`make test`, `make test-integration`, `make test-sum`, `make test-integration-sum`",
        "Targeted test coverage with optional JUnit output under `dist/test/`.",
      ],
    },
    {
      cells: [
        "Security static analysis",
        "`make semgrep-scan`, `make security-ci`",
        "Semgrep report-only scans or the full CI-equivalent security lane with vulncheck, license checks, Semgrep, and Trivy.",
      ],
    },
    {
      cells: [
        "Controller debugging",
        "`make air-controller`, `make debug-controller`, `make debug-test PKG=... TEST=...`",
        "Live reload or Delve-based debugging for controller and test work.",
      ],
    },
    {
      cells: [
        "Architecture policy",
        "`make generate-ast-rules`, `make verify-arch-policy`, `make report-internal-deps`",
        "Boundary verification plus a local dependency report for internal package drift.",
      ],
    },
  ]}
/>

<CommandBlock
  language="bash"
  label="inspect"
  title="Architecture report and policy verification"
  code={`make report-internal-deps
make report-internal-deps-snapshot
make report-internal-deps-diff

make generate-ast-rules
make verify-arch-policy
make test-ast
make lint-ast`}
>
  Use this when you change controller wiring, add new `internal/*` package boundaries, or want to compare package drift across a refactor.
</CommandBlock>

<ExpandableCallout type="question" title="RBAC errors during `make deploy`?">

Make sure the current kubeconfig identity has enough privileges to install CRDs, RBAC, and admission resources.

</ExpandableCallout>

<ExpandableCallout type="question" title="Webhooks failing locally?">

Switch from the host-based loop to the in-cluster loop. Webhook traffic needs the API server to reach the operator inside the cluster network.

</ExpandableCallout>

<ExpandableCallout type="question" title="How do I confirm my workstation is still healthy?">

Run `make doctor`. It reports missing external tools and which contributor workflows they block.

</ExpandableCallout>

<NextActions
  title="After your environment works"
  items={[
    {
      label: "Testing strategy",
      description: "Choose the smallest test layer that proves the change you are about to make.",
      to: "/contribute/testing",
    },
    {
      label: "Project conventions",
      description: "Review the repository-specific rules that shape how code, generated files, and reviews should look.",
      to: "/contribute/standards/project-conventions",
    },
    {
      label: "Continuous integration",
      description: "Open the CI map when you need to understand how local checks expand into branch and release workflows.",
      to: "/contribute/ci",
    },
  ]}
/>
