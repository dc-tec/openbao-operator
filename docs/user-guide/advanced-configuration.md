# 9. Advanced Configuration

[Back to User Guide index](README.md)

### 9.1 Audit Devices

Configure declarative audit devices for OpenBao clusters. Audit devices are created automatically on server startup and SIGHUP events.

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: audit-cluster
spec:
  # ... other spec fields ...
  audit:
    - type: file
      path: stdout
      description: "Stdout audit device for debugging"
      options:
        file_path: "/dev/stdout"
        log_raw: "true"
    - type: file
      path: secure-audit
      description: "Secure audit logging"
      options:
        file_path: "/var/log/openbao/audit.log"
        format: "json"
```

See the [OpenBao audit documentation](https://openbao.org/docs/configuration/audit/) for available audit device types and options.

### 9.2 Plugins

Configure declarative plugins for OCI-based plugin management. Plugins are automatically downloaded and registered on server startup.

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: plugin-cluster
spec:
  # ... other spec fields ...
  plugins:
    - type: secret
      name: aws
      image: "ghcr.io/openbao/openbao-plugin-secrets-aws"
      version: "v1.0.0"
      binaryName: "openbao-plugin-secrets-aws"
      sha256sum: "9fdd8be7947e4a4caf7cce4f0e02695081b6c85178aa912df5d37be97363144c"
    - type: auth
      name: kubernetes
      command: "openbao-plugin-auth-kubernetes"
      version: "v1.0.0"
      binaryName: "openbao-plugin-auth-kubernetes"
      sha256sum: "abc123..."
```

See the [OpenBao plugin documentation](https://openbao.org/docs/configuration/plugins/) for more details.

### 9.3 Telemetry

Configure telemetry reporting for metrics and observability.

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: telemetry-cluster
spec:
  # ... other spec fields ...
  telemetry:
    prometheusRetentionTime: "24h"
    disableHostname: true
    metricsPrefix: "openbao_"
    # Prometheus-specific options
    prometheusRetentionTime: "24h"
    # Or StatsD options
    # statsdAddress: "statsd.example.com:8125"
    # Or DogStatsD options
    # dogStatsdAddress: "dogstatsd.example.com:8125"
    # dogStatsdTags:
    #   - "env:production"
    #   - "cluster:main"
```

See the [OpenBao telemetry documentation](https://openbao.org/docs/configuration/telemetry/) for all available options.

### 9.4 Custom Container Images

Override default container images for air-gapped environments or custom registries:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: custom-images-cluster
spec:
  # ... other spec fields ...
  backup:
    schedule: "0 3 * * *"
    target:
      endpoint: "https://s3.amazonaws.com"
      bucket: "backups"
      region: "us-east-1"
      pathPrefix: "clusters/custom-images-cluster"
      credentialsSecretRef:
        name: s3-credentials
      # Alternative to static credentials: use Web Identity (OIDC federation).
      # When set, the backup Job mounts a projected ServiceAccount token and the
      # AWS SDK uses it via AWS_ROLE_ARN + AWS_WEB_IDENTITY_TOKEN_FILE.
      # roleArn: "arn:aws:iam::123456789012:role/openbao-backup"
    executorImage: "internal-registry.example.com/openbao/backup-executor:v0.1.0"
    # Preferred: JWT Auth (automatically rotated tokens)
    jwtAuthRole: backup
    # Alternative: Static token (for self-init clusters without JWT Auth)
    # tokenSecretRef:
    #   name: backup-token-secret
    retention:
      maxCount: 7
      maxAge: "168h"
  initContainer:
    enabled: true
    image: "internal-registry.example.com/openbao/openbao-config-init:v1.0.0"
```

**Note:** Using `:latest` tags for the init container is not recommended for production. Always specify a pinned version tag.
