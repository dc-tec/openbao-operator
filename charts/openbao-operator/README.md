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

### Multi-tenant mode with platform-managed Pod Security labels

Use this when Rancher or another platform policy layer owns namespace labels. The chart keeps tenant RBAC and quota onboarding in the operator, removes namespace update/patch RBAC from the Provisioner, and configures admission policy to deny Provisioner namespace mutations.

```yaml
tenancy:
  mode: multi
  namespacePodSecurityLabels:
    mode: external
```

### Service claims (bounded same-cluster)

The chart includes the CRDs, RBAC, webhook, and admission-policy surface needed
for `OpenBaoClusterClaim`. The runtime is feature-gated and should be enabled
explicitly only when you intend to offer the bounded same-cluster claim workflow.

```yaml
tenancy:
  mode: multi

serviceClaims:
  enabled: true
```

Use install-level defaults only when they are shared by the whole operator
installation. Prefer catalog objects such as `OpenBaoNetworkProfile` and
`OpenBaoUnsealProfile` when different offerings need different settings.

```yaml
serviceClaims:
  enabled: true
  network:
    apiServerCIDR: 10.43.0.1/32
    dnsEndpointIPs:
      - 169.254.20.10
  transitUnseal:
    address: https://transit-bao.openbao-infra.svc:8200
    keyName: openbao-unseal
    mountPath: transit
    credentialsSecretName: transit-unseal-token
```

Current claim scope is same-cluster provisioning with explicit request objects
for compatible in-place upgrades, manual backups, and restore from the latest
successful or selected completed claim backup request. Adoption, migration,
arbitrary restore-source selection, and broader multi-cluster claim convergence
are not part of this claim release.

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
