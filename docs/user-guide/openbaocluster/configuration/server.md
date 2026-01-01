# Advanced Configuration

## Prerequisites

- **OpenBao Operator**: v2.4.0+ (some features require specific versions)

## Structured Configuration

Configure OpenBao server settings using the structured `spec.configuration` API. This provides type safety and `kubectl explain` support.

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: config-cluster
spec:
  profile: Development
  # ... other spec fields ...
  configuration:
    # UI configuration
    ui: true
    # Log level (trace, debug, info, warn, err)
    logLevel: "info"
    # Logging configuration
    logging:
      format: "json"
      file: "/var/log/openbao/openbao.log"
      rotateDuration: "24h"
      rotateBytes: 10485760 # 10MB
      rotateMaxFiles: 7
    # Listener configuration
    listener:
      proxyProtocolBehavior: "use_proxy_protocol"
    # Raft storage configuration
    raft:
      performanceMultiplier: 2
    # Plugin configuration
    plugin:
      autoDownload: true
      autoRegister: false
      downloadBehavior: "direct"
    # Lease/TTL configuration
    defaultLeaseTTL: "720h" # 30 days
    maxLeaseTTL: "8760h" # 1 year
    # Cache configuration
    cacheSize: 134217728 # 128MB
    disableCache: false
    # ACME CA root (for ACME TLS mode)
    acmeCARoot: "/etc/bao/seal-creds/ca.crt"
```

**Available Configuration Options:**

- **UI**: Enable/disable the web interface
- **LogLevel**: Set log verbosity (trace, debug, info, warn, err)
- **Listener**: Configure proxy protocol behavior and TLS settings
- **Raft**: Tune Raft storage performance multiplier
- **Logging**: Configure log format, file, rotation settings, and PID file
- **Plugin**: Configure plugin file permissions, auto-download, auto-register, and download behavior
- **DefaultLeaseTTL / MaxLeaseTTL**: Configure lease TTLs
- **CacheSize / DisableCache**: Configure caching behavior
- **ACMECARoot**: Path to ACME CA root certificate (for ACME TLS mode)
- **Advanced Features**: Detect deadlocks, raw storage endpoint, introspection endpoint, disable standby reads, imprecise lease role tracking, unsafe allow API audit creation, allow audit log prefixing, enable response header hostname/raft node ID

See `config/samples/production/openbao_v1alpha1_openbaocluster_structured_config.yaml` for a comprehensive example.

### Audit Devices

Configure declarative audit devices for OpenBao clusters. Audit devices are created automatically on server startup and SIGHUP events.

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: audit-cluster
spec:
  profile: Development
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

### Plugins

Configure declarative plugins for OCI-based plugin management. Plugins are automatically downloaded and registered on server startup.

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: plugin-cluster
spec:
  profile: Development
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

### Telemetry

Configure telemetry reporting for metrics and observability.

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: telemetry-cluster
spec:
  profile: Development
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

### Custom Container Images

Override default container images for air-gapped environments or custom registries:

```yaml
apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: custom-images-cluster
spec:
  profile: Development
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
