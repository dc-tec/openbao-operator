---
title: Testing Strategy
description: Contributor testing strategy for OpenBao Operator, including test depth selection, local PR-equivalent checks, fuzzing, performance gates, and targeted cluster validation.
pageType: concept
journey: contribute
---

<PageHeader
  title="Testing strategy by change scope"
  lede="This page maps change scope to validation depth across unit, integration, E2E, fuzzing, and exploratory coverage."
/>

<DiagramFrame
  title="Testing layers"
  caption="Move to the next layer when the lower-cost layer no longer proves the behavior you changed."
  code={`graph BT
    Unit["Unit tests"]
    Contract["Fast contracts"]
    Integration["EnvTest integration"]
    E2E["Kind end-to-end"]
    Exploratory["Manual and exploratory"]

    Unit --> Contract
    Contract --> Integration
    Integration --> E2E
    E2E --> Exploratory`}
/>

<DecisionTable
  title="Choose test depth by change scope"
  columns={["If your change affects", "Run this first", "What it proves"]}
  rows={[
    {
      cells: [
        "Pure Go logic, parsers, renderers, helpers, or small decision functions",
        "`make test` or targeted `go test ./...`",
        "Deterministic in-process behavior with no Kubernetes API dependency.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Object builders, manifests, patches, or fake-client contract behavior",
        "`make test` plus targeted package tests",
        "Resource shape and emitted contracts when API-server semantics do not matter.",
      ],
    },
    {
      cells: [
        "Reconciliation, finalizers, status writes, admission behavior, or server-side validation/defaulting",
        "`make test-integration`",
        "Real API-server semantics through EnvTest.",
      ],
    },
    {
      cells: [
        "Lifecycle flows, networking, storage, upgrades, backup and restore, or workload startup",
        "`make test-e2e` or a label-filtered `make test-e2e-ci ...` run",
        "Real cluster behavior with real images and controller wiring.",
      ],
    },
    {
      cells: [
        "Disaster recovery, performance thresholds, or compatibility against an existing platform cluster",
        "Focused manual or scheduled workflow validation",
        "Evidence from the environment that production-like assumptions actually hold.",
      ],
      emphasis: "caution",
    },
  ]}
/>

<Callout type="note" title="Fake client boundary">

Use the controller-runtime fake client as a fast contract tool, not as a substitute for the API server. If the test depends on validation, defaulting, `Generation` or `ResourceVersion` behavior, subresources, watches, cache wiring, or controller-manager setup, move to EnvTest.

</Callout>

<CommandBlock
  language="bash"
  label="verify"
  title="PR-equivalent local gate"
  code={`make bootstrap
make doctor
make ci-core`}
>
  This is the standard local validation baseline before review or handoff.
</CommandBlock>

<CommandBlock
  language="bash"
  label="inspect"
  title="Focused fuzz and existing-cluster checks"
  code={`make fuzz
FUZZTIME=30s make fuzz
FUZZ_TARGET_FILTER='FuzzRenderHCL|internal/adapter/auth' make fuzz

export KUBECONFIG=/path/to/your/kubeconfig
export E2E_OPERATOR_IMAGE=quay.io/your-org/openbao-operator:dev
export E2E_API_SERVER_CIDR=0.0.0.0/0
make test-e2e-existing E2E_LABEL_FILTER='openshift'`}
>
  These commands cover parser, auth, renderer, and platform-specific validation that does not fit the default local gate.
</CommandBlock>

<DecisionTable
  title="Special validation lanes"
  columns={["Lane", "When to use it", "Primary entry point"]}
  rows={[
    {
      cells: [
        "Fuzzing",
        "Parser, renderer, auth, or normalization changes that benefit from mutated input coverage.",
        "`make fuzz` locally, `make fuzz-long` in longer sweeps.",
      ],
    },
    {
      cells: [
        "Performance",
        "Controller or lifecycle changes that may affect reconcile cost, startup time, or upgrade behavior.",
        "`make verify-perf` and the Performance Baseline Capture workflow.",
      ],
    },
    {
      cells: [
        "Existing-cluster compatibility",
        "OpenShift or platform-specific validation that a local Kind cluster cannot faithfully represent.",
        "`make test-e2e-existing ...` with a preconfigured cluster context.",
      ],
    },
  ]}
/>

<NextActions
  title="After test selection"
  items={[
    {
      label: "Continuous integration",
      description: "Continuous integration maps these validation layers to workflow gates and routing.",
      to: "/contribute/ci",
    },
    {
      label: "Coding standards",
      description: "Coding standards cover implementation cleanup when more test depth would not help yet.",
      to: "/contribute/standards",
    },
    {
      label: "Release management",
      description: "Release management covers release and publish flow once the branch is stable.",
      to: "/contribute/release-management",
    },
  ]}
/>
