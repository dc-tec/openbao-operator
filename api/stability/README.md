# API stability inventory

`v1alpha1.yaml` records the approved 0.5.0 stability decision for every user-facing
`spec` and `status` path in the three served CRDs. It is a release-planning contract,
not a declaration that `v1alpha1` is already stable.

The inventory uses inherited path rules. Every top-level field requires an explicit
rule, while nested fields inherit the closest matching rule unless a more specific
decision overrides it. The checker expands those rules against the generated CRD
schemas, including schema roots, array items, map values, composed branches, and
Kubernetes merge semantics. It rejects unclassified top-level fields and requires
every schema-deprecated field to have an explicit `deprecated` rule.

`v1alpha1-paths.tsv` is the deterministic resolved contract. It ensures nested fields
cannot appear, disappear, or change their schema facts or inherited decision without a
reviewable snapshot diff.

The resolved review table derives field purpose, requiredness, defaults, and structural
validation directly from the generated schema. The policy file adds ownership,
semantic omission behavior where it differs from the schema, mutability, stable status
values, enforcement layers, module interaction, migration posture, and the approved
stability decision. `grow-only` and `transition-guarded` distinguish constrained writes
from freely mutable fields; `request-token` identifies changed values that trigger a
one-shot operator action.

Classifications have the following meanings:

- `beta-stable`: intended to retain its name, type, omission/default semantics,
  meaning, and mutability under the approved 0.5.0 freeze
- `additive-unfrozen`: supported current surface whose beta commitment still needs
  a later decision; compatible additions may continue
- `likely-move`: known candidate for validation or shape work before beta
- `deprecated`: compatibility field with a documented replacement
- `stable-automation`: status intended for machine consumers; message prose and
  expanding reason sets remain outside this guarantee
- `informational`: status intended for observation and debugging rather than a
  durable automation contract

Run the completeness check with:

```sh
make verify-api-stability-inventory
```

After an intentional inventory or CRD change, update and review the resolved snapshot:

```sh
make update-api-stability-inventory
```

Render the fully resolved field table for review with:

```sh
go run ./hack/tools/api_inventory --format markdown
```

## CRD compatibility report

`baselines/0.4.2.json` is a normalized snapshot of the CRDs shipped in the
`0.4.2` release asset. The snapshot records the source asset SHA-256 digest so
the comparison remains tied to what users installed rather than to a mutable
source checkout.

Run the report with:

```sh
make report-crd-compatibility
```

The 0.5.0 stabilization cycle runs this check in report-only mode so intentional
pre-beta removals and validation tightening stay visible without blocking the
work. After 0.5.0 is published, generate a baseline from its released
`crds.yaml` asset and switch `CRD_COMPAT_MODE` to `enforce` so breaking or
review-required changes fail CI by default.
