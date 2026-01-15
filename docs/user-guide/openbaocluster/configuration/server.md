# Advanced Configuration

The `OpenBaoCluster` Custom Resource provides comprehensive configuration options for the OpenBao server.

## Configuration Groups

=== "Core"
    Configure the fundamental server behaviors like UI, Listening, and Storage tuning via `spec.configuration`.

    ```yaml
    spec:
      configuration:
        # User Interface
        ui: true
        
        # Performance Tuning
        cacheSize: 134217728  # 128MB
        disableCache: false
        
        # Raft Storage
        raft:
          performanceMultiplier: 2
          # Autopilot (enabled by default)
          autopilot:
            cleanupDeadServers: true
            deadServerLastContactThreshold: "5m"
            minQuorum: 2
        
        # Lease Management
        defaultLeaseTTL: "720h" # 30 days
        maxLeaseTTL: "8760h"    # 1 year
        
        # Listener Settings
        listener:
          proxyProtocolBehavior: "use_proxy_protocol"
    ```

    | Field | Description |
    | :--- | :--- |
    | `ui` | Enable/Disable the web interface. |
    | `listener` | Configure TLS and Proxy Protocol usage. |
    | `raft.performanceMultiplier` | Tune Raft timing for high-latency environments. |
    | `raft.autopilot` | Configure Autopilot dead server cleanup (enabled by default). |
    | `defaultLeaseTTL` | Default Time-To-Live for leases. |

=== "Observability"
    Configure Logging and Telemetry for monitoring.

    ```yaml
    spec:
      configuration:
        logLevel: "info"
        logging:
          format: "json"
          file: "/var/log/openbao/openbao.log"
          rotateDuration: "24h"
          rotateMaxFiles: 7
          
      telemetry:
        prometheusRetentionTime: "24h"
        disableHostname: true
        metricsPrefix: "openbao_"
    ```
    
    See [Telemetry Documentation](https://openbao.org/docs/configuration/telemetry/) for all provider options (StatsD, DogStatsD, etc.).

=== "Security"
    Configure declarative **Audit Devices**. These are automatically enabled on startup.

    ```yaml
    spec:
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

=== "Plugins"
    Configure OCI-based **Plugins** which are automatically downloaded and registered.

    ```yaml
    spec:
      configuration:
        plugin:
          autoDownload: true
          downloadBehavior: "direct"
          
      plugins:
        - type: secret
          name: aws
          image: "ghcr.io/openbao/openbao-plugin-secrets-aws"
          version: "v1.0.0"
          binaryName: "openbao-plugin-secrets-aws"
          sha256sum: "9fdd8be7947e4a4caf7cce4f0e02695081b6c85178aa912df5d37be97363144c"
    ```

=== "Operations"
    Configure **Images**, **Backups**, and **Init Containers**.

    ```yaml
    spec:
      # Override Images for Air-Gapped Environments
      image: "internal-registry.example.com/openbao/openbao:2.4.0"
      
      initContainer:
        enabled: true
        image: "internal-registry.example.com/openbao/openbao-config-init:v1.0.0"

      backup:
        executorImage: "internal-registry.example.com/openbao/backup-executor:v0.1.0"
        schedule: "0 3 * * *"
        retention:
          maxCount: 7
        target:
          endpoint: "https://s3.amazonaws.com"
          bucket: "backups"
          region: "us-east-1"
          credentialsSecretRef:
            name: s3-credentials
    ```

## Feature Reference

For a complete list of `spec.configuration` fields, run:

```bash
kubectl explain openbaocluster.spec.configuration
```
