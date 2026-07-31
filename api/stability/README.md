# API stability inventory

`v1alpha1.yaml` records the proposed 0.5.0 stability decision for every user-facing
`spec` and `status` path in the three served CRDs. It is a release-planning contract,
not a declaration that `v1alpha1` is already stable.

The inventory uses inherited path rules. Every top-level field requires an explicit
rule, while nested fields inherit the closest matching rule unless a more specific
decision overrides it. The checker expands those rules against the generated CRD
schemas, rejects unclassified top-level fields, and requires every schema-deprecated
field to have an explicit `deprecated` rule.

`v1alpha1-paths.tsv` is the deterministic resolved contract. It ensures nested fields
cannot appear, disappear, or change their schema facts or inherited decision without a
reviewable snapshot diff.

The resolved review table derives field purpose, requiredness, defaults, and structural
validation directly from the generated schema. The policy file adds ownership,
mutability, enforcement layers, module interaction, migration posture, and the proposed
stability decision.

Classifications have the following meanings:

- `beta-stable`: intended to retain its name, type, omission/default semantics,
  meaning, and mutability after the 0.5.0 freeze is approved
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
