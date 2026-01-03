# Go Style Guide

General Go coding style and conventions for the OpenBao Operator.

## 1. Naming Conventions

### Acronyms

Keep acronyms consistent in casing. Avoid "Java-style" mixed capitalization for acronyms.

=== ":material-check: Good Pattern"
    ```go
    // Keep acronyms all-caps
    ServeHTTP
    NewUUID
    ParseURL
    userID
    ```

=== ":material-close: Bad Pattern"
    ```go
    // Do not mix case in acronyms
    ServeHttp
    NewUuid
    ParseUrl
    userId
    ```

### Getters

Go prefers direct naming for getters. Do NOT prefix with `Get`.

=== ":material-check: Good Pattern"
    ```go
    func (c *Cluster) Name() string
    func (c *Cluster) Status() string
    ```

=== ":material-close: Bad Pattern"
    ```go
    func (c *Cluster) GetName() string
    func (c *Cluster) GetStatus() string
    ```

### Interfaces

Interfaces with a single method should end in `-er`.

=== ":material-check: Good Pattern"
    ```go
    type Reader interface { Read(p []byte) (n int, err error) }
    type CertRotator interface { Rotate(ctx context.Context) error }
    ```

## 2. Error Handling

### Wrapping

Always wrap errors to preserve context using `%w`.

=== ":material-check: Good Pattern"
    ```go
    if err != nil {
        return fmt.Errorf("failed to sync secret: %w", err)
    }
    ```

=== ":material-close: Bad Pattern"
    ```go
    if err != nil {
        // Context is lost
        return err
    }
    ```

### Sentinel Errors

Define exported sentinel errors for conditions callers might need to check.

```go
var (
    ErrClusterNotReady = errors.New("cluster not ready")
    ErrSecretNotFound  = errors.New("secret not found")
)
```

## 3. Structured Logging

Use `logr` with key-value pairs. Never use `fmt.Printf` or `log.Println`.

=== ":material-check: Good Pattern"
    ```go
    log.Info("Reconciling Cluster",
        "namespace", req.Namespace,
        "name", req.Name,
    )
    ```

=== ":material-close: Bad Pattern"
    ```go
    // Unstructured and lacks context
    log.Info(fmt.Sprintf("Reconciling Cluster %s/%s", req.Namespace, req.Name))

    // Forbidden
    fmt.Printf("Reconciling %s\n", req.Name)
    ```

!!! danger "Security Warning"
    **NEVER** log secrets, tokens, keys, or passwords. Even in debug mode.

## 4. Concurrency & Reconcilers

### No Goroutines in Reconcile

The `Reconcile` loop is already concurrent (if configured). Do not spawn unmanaged goroutines.

=== ":material-check: Good Pattern"
    ```go
    // Do work synchronously
    if err := r.ensurePods(ctx, cluster); err != nil {
        return ctrl.Result{}, err
    }
    ```

=== ":material-close: Bad Pattern"
    ```go
    // Fails silently, loses context, risks race conditions
    go func() {
        r.ensurePods(ctx, cluster)
    }()
    ```

### No `time.Sleep`

Blocking a reconciler thread degrades the entire controller's performance.

=== ":material-check: Good Pattern"
    ```go
    // Requeue nicely
    return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
    ```

=== ":material-close: Bad Pattern"
    ```go
    // Blocks the worker thread
    time.Sleep(5 * time.Second)
    ```

## 5. Imports

Group imports into three blocks separated by newlines:

1. Standard Library
2. Third-party (e.g., K8s, Controller Runtime)
3. Local (`github.com/dc-tec/openbao-operator/...`)

```go
import (
    "context"
    "fmt"

    appsv1 "k8s.io/api/apps/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"

    "github.com/dc-tec/openbao-operator/internal/config"
)
```

## 6. Constants

Avoid "Magic Numbers" or raw strings in logic.

=== ":material-check: Good Pattern"
    ```go
    const (
        DefaultReplicas   = 3
        TLSSecretSuffix   = "-tls"
        ReconcileInterval = 10 * time.Second
    )
    ```

=== ":material-close: Bad Pattern"
    ```go
    // What does "3" mean here?
    if cluster.Spec.Replicas < 3 { ... }

    // Hardcoded strings are prone to typos
    secretName := cluster.Name + "-tls"
    ```
