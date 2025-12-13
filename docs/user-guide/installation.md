# 1. Deploy the Operator

[Back to User Guide index](README.md)

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
