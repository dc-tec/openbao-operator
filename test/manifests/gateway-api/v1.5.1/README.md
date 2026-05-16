# Gateway API manifests (v1.5.1)

This directory vendors the Kubernetes Gateway API CRDs for use in local and CI testing.

- Version: `v1.5.1`
- Source: https://github.com/kubernetes-sigs/gateway-api/tree/v1.5.1/config/crd/standard
- Used by: `envtest` suites (unit/integration tests that require Gateway API types)

## Contents

- `crds/`: CRD-only YAMLs copied from the upstream Standard channel.

## Updating

If you bump the Gateway API version:

1. Replace the CRD YAMLs under `crds/` with the new upstream Standard channel version.
2. Update any envtest suites that reference this path.
3. Run `make test-ci` to validate envtest still boots and Gateway resources can be created.
