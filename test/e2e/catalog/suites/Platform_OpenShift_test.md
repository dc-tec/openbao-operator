# OpenShift Platform

Source: `test/e2e/Platform_OpenShift_test.go`

Note: recorded checkpoints are best-effort extracts from literal `By(...)` calls visible to `ginkgo outline`.

## Cases

| Case ID | Spec | State | Covers | Labels |
| --- | --- | --- | --- | --- |
| `platform-openshift-creates-statefulset-pod-template-without-pinned-30198dd7` | creates StatefulSet pod template without pinned UID/GID/FSGroup | active | _none_ | `openshift`, `platform` |
| `platform-openshift-does-not-pin-runasuser-runasgroup-in-ee682f26` | does not pin runAsUser/runAsGroup in operator Deployments | active | _none_ | `openshift`, `platform` |

## `platform-openshift-creates-statefulset-pod-template-without-pinned-30198dd7`

Path: `OpenShift Platform > creates StatefulSet pod template without pinned UID/GID/FSGroup`

State: `active`

Covers: _none_

Labels: `openshift`, `platform`

Recorded checkpoints:
- creating a development cluster on OpenShift
- verifying the StatefulSet pod template leaves UID, GID, and FSGroup unpinned


## `platform-openshift-does-not-pin-runasuser-runasgroup-in-ee682f26`

Path: `OpenShift Platform > does not pin runAsUser/runAsGroup in operator Deployments`

State: `active`

Covers: _none_

Labels: `openshift`, `platform`


