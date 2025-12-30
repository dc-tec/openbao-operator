---
description: Deploy the operator to a local Kind cluster
---
This workflow sets up a local development environment and deploys the OpenBao Operator.

# Setup Cluster

Ensure you have a Kind cluster running. This command will create one if it doesn't exist:

// turbo

```bash
make setup-test-e2e
```

# Deploy Operator

Build and deploy the operator to the current cluster:

```bash
make deploy
```

# Run Locally (Alternative)

If you prefer to run the controller locally outside the cluster (against the cluster):

```bash
make run-controller
```
