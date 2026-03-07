---
description: Debug the operator locally with Air, Delve, and Tilt
---

Use this workflow when you need a fast local edit-run loop, interactive debugging, or a cluster-native repro.

# Fast local restart loop

For controller changes against the current kubeconfig:

```bash
make air-controller
```

For provisioner changes:

```bash
make air-provisioner
```

# Interactive debugging with Delve

Debug the controller:

```bash
make debug-controller
```

Debug the provisioner:

```bash
make debug-provisioner
```

Debug a specific Go test package:

```bash
make debug-test PKG=./internal/... TEST=TestName
```

# Switch to a cluster-native repro when needed

If the issue depends on webhooks, RBAC, image behavior, or multi-resource logs, use Tilt instead of a local binary:

```bash
make tilt-up
```

Stop the Tilt session when finished:

```bash
make tilt-down
```
