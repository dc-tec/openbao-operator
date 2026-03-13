# OpenBao Operator Helm Chart

![OpenBao Operator logo](https://raw.githubusercontent.com/dc-tec/openbao-operator/main/docs/assets/repo_logo.png)

This chart installs the OpenBao Operator and its cluster-scoped dependencies.

## Prerequisites

- Kubernetes `>=1.33.0-0`
- Helm `v3` or `v4`

## Install

```bash
kubectl create namespace openbao-operator-system

helm upgrade --install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
  --version <chart-version> \
  --namespace openbao-operator-system
```

## Common Configuration

### Multi-tenant mode (default)

```yaml
tenancy:
  mode: multi
```

### Single-tenant mode

```yaml
tenancy:
  mode: single
  targetNamespace: openbao-system
```

### Single-tenant mode with custom Helm identity

Use the release name or `fullnameOverride` when you want a custom operator identity. The chart keeps the controller `ServiceAccount`, single-tenant `RoleBinding`, and admission-policy references aligned from the rendered fullname.

```yaml
fullnameOverride: team-bao-operator

tenancy:
  mode: single
  targetNamespace: openbao-system
```

Apply with:

```bash
helm upgrade --install openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
  --version <chart-version> \
  --namespace openbao-operator-system \
  --values values.yaml
```

## Upgrade

```bash
helm upgrade openbao-operator oci://ghcr.io/dc-tec/charts/openbao-operator \
  --version <chart-version> \
  --namespace openbao-operator-system
```

## Uninstall

```bash
helm uninstall openbao-operator --namespace openbao-operator-system
```

## Values Reference

See:

- `charts/openbao-operator/values.yaml`
- `charts/openbao-operator/values.schema.json`

## More Information

- Documentation: https://dc-tec.github.io/openbao-operator/latest/
- Compatibility Matrix: https://dc-tec.github.io/openbao-operator/latest/reference/compatibility/
- Source: https://github.com/dc-tec/openbao-operator
