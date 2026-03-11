# Security: Anti-Tamper Policy

Source: `test/e2e/anti_tamper_policy_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `anti-tamper-configmap-delete-blocked` | prevents deletion of the managed ConfigMap | active | `anti-tamper`, `configmap-protection` | `security`, `tamper`, `cluster`, `slow` |
| `anti-tamper-statefulset-delete-blocked` | prevents deletion of the managed StatefulSet | active | `anti-tamper`, `statefulset-protection` | `security`, `tamper`, `cluster`, `slow` |

## `anti-tamper-configmap-delete-blocked`

Path: `Security: Anti-Tamper Policy > managed-resource mutation guardrails > prevents deletion of the managed ConfigMap`

State: `active`

Generated fallback ID: `anti-tamper-policy-prevents-deletion-of-the-managed-configmap-19c6bc2c`

Covers: `anti-tamper`, `configmap-protection`

Labels: `security`, `tamper`, `cluster`, `slow`


## `anti-tamper-statefulset-delete-blocked`

Path: `Security: Anti-Tamper Policy > managed-resource mutation guardrails > prevents deletion of the managed StatefulSet`

State: `active`

Generated fallback ID: `anti-tamper-policy-prevents-deletion-of-the-managed-statefulset-063be7a4`

Covers: `anti-tamper`, `statefulset-protection`

Labels: `security`, `tamper`, `cluster`, `slow`


