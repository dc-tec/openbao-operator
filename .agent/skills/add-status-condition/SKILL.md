# Skill: add-status-condition

Use this skill when you need to add or update an `OpenBaoCluster` Status Condition.

## Checklist

1. Identify the failing behavior and the reconciler phase that observes it.
2. Add a typed/sentinel error (or an explicit branch) in the producer layer (infra/config/certs) if the controller must react differently.
3. Map the error/branch to:
   - Condition `Type`
   - `Status` (`True/False/Unknown`)
   - `Reason` (stable, PascalCase)
   - Human-readable `Message` with remediation
4. Add unit coverage:
   - Condition helper tests (preferred)
   - Or reconciler unit tests asserting Condition transitions
5. Update troubleshooting docs when the condition is user-facing.
