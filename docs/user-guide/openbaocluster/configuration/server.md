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
            # minQuorum is automatically calculated based on profile and replicas
            # Hardened: max(3, replicas) - ensures HA safety
            # Development: replicas (minimum 1) - allows single-node clusters
            # minQuorum: 3  # Optional: override automatic calculation
            serverStabilizationTime: "10s"
        
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
    | `raft.autopilot` | Configure Autopilot dead server cleanup (enabled by default). See [Autopilot Configuration](#autopilot-configuration) below. |
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

## Autopilot Configuration

Raft Autopilot automatically removes dead servers to properly manage the Raft quorum. The Operator configures this automatically based on your `spec.profile` and `spec.replicas`.

!!! note "Automatic Reconciliation"
    The operator updates Autopilot settings whenever `spec.replicas` changes, ensuring `min_quorum` remains safe.

### Default Values

The operator sets safe defaults based on your cluster profile:

| Setting | Hardened Profile | Development Profile | Description |
| :--- | :--- | :--- | :--- |
| `cleanupDeadServers` | `true` | `true` | Enable automatic removal of failed peers |
| `deadServerLastContactThreshold` | `5m` | `5m` | Time before a server is considered dead (shorter than OpenBao's 24h default for K8s) |
| `lastContactThreshold` | `10s` | `10s` | Time without leader contact before a server is considered unhealthy |
| `maxTrailingLogs` | `1000` | `1000` | Maximum Raft log entries a server can be behind before being considered unhealthy |
| `serverStabilizationTime` | `10s` | `10s` | Minimum time a server must be healthy before promotion to voter |
| `minQuorum` | `max(3, replicas)` | `replicas` (min 1) | Minimum servers before pruning allowed |

### Automatic `min_quorum` Calculation

The operator automatically calculates `min_quorum` based on your cluster profile and replica count to ensure safety or flexibility.

=== "Hardened Profile"

    Designed for High Availability and production safety.

    - **Calculation:** `max(3, replicas)`
    - **Behavior:** Ensures at least 3 servers are required for quorum pruning. If you scale to 5 replicas, it safely adjusts to 5.
    - **Goal:** Prevent split-brain scenarios and data loss.

=== "Development Profile"

    Designed for resource efficiency and single-node testing.

    - **Calculation:** `replicas` (min: 1)
    - **Behavior:** Allows single-node clusters (`min_quorum=1`) and small multi-node setups without strict HA requirements.
    - **Goal:** Minimize resource usage for non-production environments.

### Customization

Override any default value via `spec.configuration.raft.autopilot`:

```yaml
spec:
  profile: Hardened
  replicas: 5
  configuration:
    raft:
      autopilot:
        # Override automatic min_quorum calculation
        minQuorum: 4
        # Customize dead server threshold
        deadServerLastContactThreshold: "10m"
        # Customize unhealthy server threshold
        lastContactThreshold: "30s"
        # Customize max trailing logs
        maxTrailingLogs: 2000
        # Customize stabilization time
        serverStabilizationTime: "30s"
```

!!! tip "When to Override min_quorum"
    Generally, you should let the operator calculate `min_quorum` automatically. Override only if you have specific requirements, such as maintaining a higher quorum during maintenance windows.

### Disabling Autopilot Cleanup

To disable automatic dead server cleanup:

```yaml
spec:
  configuration:
    raft:
      autopilot:
        cleanupDeadServers: false
```

!!! warning "Manual Cleanup Required"
    When disabled, you must manually remove dead Raft peers via `bao operator raft remove-peer` or the OpenBao API.

## Feature Reference

For a complete list of `spec.configuration` fields, run:

```bash
kubectl explain openbaocluster.spec.configuration
```
