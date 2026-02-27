---
description: Sync CRD changes from api/ to Helm chart. Run after modifying api/v1alpha1/ types.
---

# Sync CRDs and Helm Chart

Run this workflow after making changes to `api/v1alpha1/` types to propagate changes to CRDs and Helm chart.

1. Generate CRD manifests from Go types:

```sh
make manifests
```

1. Generate DeepCopy methods:

```sh
make generate
```

1. Sync CRDs and admission policies to Helm chart:

```sh
make helm-sync
```

1. Lint the Helm chart:

```sh
make helm-lint
```

1. Test Helm chart templating:

```sh
make helm-test
```

1. Verify no unexpected changes:

```sh
git diff --stat
```
