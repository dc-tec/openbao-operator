# Claims Guardrails

Source: `test/e2e/Claims_Guardrails_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `claims-guardrails-child-delete` | denies direct deletion of a claim-managed local OpenBaoCluster | active | `claim-managed-child-protection` | `claims`, `claims-guardrails`, `security`, `admission` |
| `claims-guardrails-offering-pin` | denies materialized claim spec mutation | active | `claim-offering-pin` | `claims`, `claims-guardrails`, `security`, `admission` |
| `claims-guardrails-spec-lock` | denies materialized claim spec mutation | active | `claim-spec-lock` | `claims`, `claims-guardrails`, `security`, `admission` |
| `claims-guardrails-materialize` | materializes a same-cluster claim before guardrail checks | active | `claim-materialization` | `claims`, `claims-guardrails`, `security`, `admission` |

## `claims-guardrails-child-delete`

Path: `Claims Guardrails > denies direct deletion of a claim-managed local OpenBaoCluster`

State: `active`

Generated fallback ID: `claims-guardrails-denies-direct-deletion-of-a-claim-e5819b0b`

Covers: `claim-managed-child-protection`

Labels: `claims`, `claims-guardrails`, `security`, `admission`


## `claims-guardrails-offering-pin`

Path: `Claims Guardrails > denies materialized claim spec mutation`

State: `active`

Generated fallback ID: `claims-guardrails-denies-materialized-claim-spec-mutation-b031911a`

Covers: `claim-offering-pin`

Labels: `claims`, `claims-guardrails`, `security`, `admission`


## `claims-guardrails-spec-lock`

Path: `Claims Guardrails > denies materialized claim spec mutation`

State: `active`

Generated fallback ID: `claims-guardrails-denies-materialized-claim-spec-mutation-b031911a`

Covers: `claim-spec-lock`

Labels: `claims`, `claims-guardrails`, `security`, `admission`


## `claims-guardrails-materialize`

Path: `Claims Guardrails > materializes a same-cluster claim before guardrail checks`

State: `active`

Generated fallback ID: `claims-guardrails-materializes-a-same-cluster-claim-before-f4aeb3ec`

Covers: `claim-materialization`

Labels: `claims`, `claims-guardrails`, `security`, `admission`
