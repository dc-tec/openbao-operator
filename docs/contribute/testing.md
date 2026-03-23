---
title: Testing Strategy
description: Contributor testing strategy for OpenBao Operator, including test depth selection, local PR-equivalent checks, fuzzing, performance gates, and targeted cluster validation.
pageType: concept
journey: contribute
---

<PageHero
  variant="compact"
  eyebrow="Contribute / Validate & Ship"
  title="Choose the smallest test that proves the change you actually made."
  lede="The testing stack is layered on purpose. Start with the cheapest signal that can fail for the right reason, then move outward only when the change touches controller wiring, real API semantics, full cluster lifecycle, or environment-specific behavior."
  actions={[
    {label: "Open CI behavior", to: "/contribute/ci", variant: "primary"},
    {label: "Set up your environment", to: "/contribute/getting-started/development", variant: "secondary"},
  ]}
>
  <Checklist
    title="Use this page when you need to"
    items={[
      "map a change to the right unit, contract, integration, E2E, or manual validation layer",
      "run the local PR-equivalent gate before you ask CI to prove the same thing again",
      "understand when fake clients are sufficient and when EnvTest or a real cluster is required",
      "replay fuzz, performance, or existing-cluster checks outside the default local path",
    ]}
  />
</PageHero>

<DiagramFrame
  title="Testing layers"
  caption="Move outward only when the cheaper layer can no longer prove the behavior you changed."
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
  This is the default maintainer expectation before you open a PR or ask someone else to chase a failing branch.
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
  Use fuzzing when parsers, normalizers, auth helpers, or config rendering changed. Use `test-e2e-existing` when you need focused OpenShift or non-Kind validation against an existing cluster.
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
      description: "See how CI maps these layers to workflow gates, routing, and label-driven E2E expansion.",
      to: "/contribute/ci",
    },
    {
      label: "Coding standards",
      description: "Go back to project conventions when the implementation itself still needs cleanup before more test depth helps.",
      to: "/contribute/standards",
    },
    {
      label: "Release management",
      description: "Move into release and publish flow once the branch is stable and ready for maintainer handling.",
      to: "/contribute/release-management",
    },
  ]}
/>
