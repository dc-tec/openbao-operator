# 17. Next Steps

[Back to User Guide index](README.md)

Once you have basic cluster creation working:

- Experiment with `spec.config` to inject additional, non-protected OpenBao configuration (validated via allowlist).
- Configure audit devices via `spec.audit` for declarative audit logging.
- Configure plugins via `spec.plugins` for OCI-based plugin management.
- Configure telemetry via `spec.telemetry` for metrics and observability.
- Configure `spec.backup` and object storage to enable snapshot streaming (see the TDD for details).
- Integrate Operator metrics with your monitoring stack using the manifests under `config/prometheus`.

For deeper architectural details and the full roadmap (upgrades, backups, multi-tenancy),
refer to [Architecture](../architecture.md) and [Security](../security.md).
