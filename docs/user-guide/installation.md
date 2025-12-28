# Operator Deployment
<!-- id: installation-guide -->

## Prerequisites

- **Kubernetes Cluster**: v1.23+
- **kubectl**: Installed and configured
- **Permissions**: Cluster-admin permissions to install CRDs and RBAC

## Deploy the Operator

Build and push an image:

```sh
make docker-build docker-push IMG=<your-registry>/openbao-operator:tag
```

Install CRDs and deploy the manager:

```sh
make install
make deploy IMG=<your-registry>/openbao-operator:tag
```

Verify the controller is running:

```sh
kubectl -n openbao-operator-system get pods
```

Adjust the namespace above if you changed the default deployment namespace.
