# Go Style Guide

General Go coding style and conventions for the OpenBao Operator.

## 1. Naming Conventions

### Acronyms

Keep acronyms consistent in casing. Avoid "Java-style" mixed capitalization for acronyms.

<Tabs groupId="good-pattern-bad-pattern">

<TabItem value="good-pattern" label="Good Pattern">

```go
// Keep acronyms all-caps
ServeHTTP
NewUUID
ParseURL
userID
```

</TabItem>

<TabItem value="bad-pattern" label="Bad Pattern">

```go
// Do not mix case in acronyms
ServeHttp
NewUuid
ParseUrl
userId
```

</TabItem>

</Tabs>

### Getters

Go prefers direct naming for getters. Do NOT prefix with `Get`.

<Tabs groupId="good-pattern-bad-pattern">

<TabItem value="good-pattern" label="Good Pattern">

```go
func (c *Cluster) Name() string
func (c *Cluster) Status() string
```

</TabItem>

<TabItem value="bad-pattern" label="Bad Pattern">

```go
func (c *Cluster) GetName() string
func (c *Cluster) GetStatus() string
```

</TabItem>

</Tabs>

### Interfaces

Interfaces with a single method should end in `-er`.

<Tabs groupId="good-pattern">

<TabItem value="good-pattern" label="Good Pattern">

```go
type Reader interface { Read(p []byte) (n int, err error) }
type CertRotator interface { Rotate(ctx context.Context) error }
```

</TabItem>

</Tabs>

## 2. Error Handling

### Wrapping

Always wrap errors to preserve context using `%w`.

<Tabs groupId="good-pattern-bad-pattern">

<TabItem value="good-pattern" label="Good Pattern">

```go
if err != nil {
    return fmt.Errorf("failed to sync secret: %w", err)
}
```

</TabItem>

<TabItem value="bad-pattern" label="Bad Pattern">

```go
if err != nil {
    // Context is lost
    return err
}
```

</TabItem>

</Tabs>

### Checkable Errors

Define exported, well-known errors for conditions callers might need to check.

```go
var (
    ErrClusterNotReady = errors.New("cluster not ready")
    ErrSecretNotFound  = errors.New("secret not found")
)
```

## 3. Structured Logging

Use `logr` with key-value pairs. Never use `fmt.Printf` or `log.Println`.

<Tabs groupId="good-pattern-bad-pattern">

<TabItem value="good-pattern" label="Good Pattern">

```go
log.Info("Reconciling Cluster",
    "cluster_namespace", req.Namespace,
    "cluster_name", req.Name,
)
```

</TabItem>

<TabItem value="bad-pattern" label="Bad Pattern">

```go
// Unstructured and lacks context
log.Info(fmt.Sprintf("Reconciling Cluster %s/%s", req.Namespace, req.Name))

// Forbidden
fmt.Printf("Reconciling %s\n", req.Name)
```

</TabItem>

</Tabs>

<Callout type="danger" title="Security Warning">

**NEVER** log secrets, tokens, keys, or passwords. Even in debug mode.

</Callout>

## 4. Concurrency & Reconcilers

### No Goroutines in Reconcile

The `Reconcile` loop is already concurrent (if configured). Do not spawn unmanaged goroutines.

<Tabs groupId="good-pattern-bad-pattern">

<TabItem value="good-pattern" label="Good Pattern">

```go
// Do work synchronously
if err := r.ensurePods(ctx, cluster); err != nil {
    return ctrl.Result{}, err
}
```

</TabItem>

<TabItem value="bad-pattern" label="Bad Pattern">

```go
// Fails silently, loses context, risks race conditions
go func() {
    r.ensurePods(ctx, cluster)
}()
```

</TabItem>

</Tabs>

### No `time.Sleep`

Blocking a reconciler thread degrades the entire controller's performance.

<Tabs groupId="good-pattern-bad-pattern">

<TabItem value="good-pattern" label="Good Pattern">

```go
// Requeue nicely
return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
```

</TabItem>

<TabItem value="bad-pattern" label="Bad Pattern">

```go
// Blocks the worker thread
time.Sleep(5 * time.Second)
```

</TabItem>

</Tabs>

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

    "github.com/dc-tec/openbao-operator/internal/adapter/config"
)
```

## 6. Constants

Avoid "Magic Numbers" or raw strings in logic.

<Tabs groupId="good-pattern-bad-pattern">

<TabItem value="good-pattern" label="Good Pattern">

```go
const (
    DefaultReplicas   = 3
    TLSSecretSuffix   = "-tls"
    ReconcileInterval = 10 * time.Second
)
```

</TabItem>

<TabItem value="bad-pattern" label="Bad Pattern">

```go
// What does "3" mean here?
if cluster.Spec.Replicas < 3 { ... }

// Hardcoded strings are prone to typos
secretName := cluster.Name + "-tls"
```

</TabItem>

</Tabs>

