# VAP Hardening (Drift Detector Removed)

This document captures the rationale and the shipped changes to rely on Validating Admission Policies
(VAPs) for drift prevention and remove the drift detector component.

## Goals

- Block meaningful drift of OpenBao Operator-managed resources at admission time (GitOps-safe).
- Preserve compatibility with common cluster add-ons (cloud load balancers, kube controllers, GC).
- Reduce maintenance burden by removing the drift detector component.
- Keep the operator usable in both single-tenant and multi-tenant modes.

## Non-Goals

- Guarantee drift correction latency of seconds (the drift detector's main value-add).
- Support every third-party controller mutating operator-managed objects; only Kubernetes-required
  flows and widely deployed infrastructure controllers are in scope.
- Execute CEL expressions in unit tests (we rely on integration/e2e for behavioral validation).

!!! note
    The drift detector only triggers on **successful** mutations that reach etcd. When drift is blocked at admission
    time, it provides little practical value beyond faster reconciliation.

## Current State (Summary)

- The primary drift-blocking policy is `config/policy/lock-managed-resource-mutations.yaml`.
- This policy must be strict enough to deny meaningful drift while allowing required Kubernetes
  controller behavior (e.g., Service finalizers/annotations for LoadBalancers).

## Compatibility Targets

The policy changes must keep these scenarios working:

1. **Namespace deletion and garbage collection**
   - Kubernetes controllers must be able to delete objects and remove finalizers.
2. **Service and Ingress/Gateway status management**
   - Infrastructure controllers must be able to update `*/status`.
3. **LoadBalancer Service lifecycle**
   - Cloud provider controllers may add/remove finalizers and set certain annotations.
4. **Pod lifecycle and kubelet deletes**
   - Kubelet and controllers must be able to create/delete/update Pods as required.
5. **OpenBao self-observation via Pod labels**
   - Pods update a small set of “service registration” labels used for status aggregation.

!!! warning
    The most common compatibility pitfall is `Service` finalizers and controller-added annotations for
    `ServiceType=LoadBalancer`. These are not `/status` writes and will be blocked if not explicitly allowed.

## Design: Narrow, Resource-Specific Exceptions

### Guiding Principle

Replace broad identity-based bypasses with **resource- and field-scoped** allowances.

At a high level, `lock-managed-resource-mutations` should enforce:

- Default: deny all `CREATE`/`UPDATE`/`DELETE` to operator-managed objects.
- Allow the OpenBao Operator controller identity to manage its objects.
- Allow Kubernetes system controllers only for operations they must perform (status, deletes, finalizers).
- Avoid allowing any non-operator actor to change `spec` (for resources where `spec` is meaningful).

### Policy Changes (Primary)

Target: `config/policy/lock-managed-resource-mutations.yaml`.

1. **Remove blanket bypasses**
   - Remove unconditional `variables.is_kube_system_controller` allow in the main validation.
   - Replace unconditional `variables.is_cert_manager` allow with a Secrets-only allowance.

2. **Allow narrowly-scoped Service metadata updates**
   - Permit kube-system identities to update `Service` metadata while enforcing:
     - `spec` is unchanged
     - labels and ownerRefs are unchanged
     - `openbao.org/*` annotations are immutable (to avoid escaping operator intent)

3. **Keep existing safe allowances**
   - Keep endpoint-related allowances (`is_endpoints_controller_request`) for system controllers.
   - Keep Pod deletion allowance for kubelet nodes.
   - Keep the pod “service registration label update” allowance for cluster pods, but validate it remains
     strictly label-scoped.

## Changes Implemented

- Removed drift detector feature (code, CRD fields, manifests, and tests).
- Tightened `lock-managed-resource-mutations` to remove broad identity bypasses while allowing required
  kube-system Service metadata updates and Secrets-only cert-manager writes.

## Rollout Strategy

- Prefer running integration/e2e in representative environments (cloud LB, bare metal, and local Kind)
  to validate Service controller behavior.

## Acceptance Criteria

- Drift attempts against operator-managed `StatefulSet`/`ConfigMap`/`Service` are denied at admission time.
- LoadBalancer and service discovery still function in common environments.
- No drift detector is required to maintain correctness.

## Local Verification (PR-Equivalent)

```sh
make lint-config lint
make verify-fmt
make verify-tidy
make verify-generated
make verify-helm
make test-ci
```
