---
description: Lint code and verify linters configuration
---
This workflow helps maintain code quality by running linters and verifying configurations.

# Lint and Fix

To run `golangci-lint` and automatically fix common issues:

```bash
make lint-fix
```

# Just Lint

To run the linter without modifying files:

```bash
make lint
```

# Verify Config

To verify the `golangci-lint` configuration:

```bash
make lint-config
```
