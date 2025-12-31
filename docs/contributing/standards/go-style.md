# Go Style Guide

General Go coding style and conventions for the OpenBao Operator.

## Formatting

- **All code must be formatted** with `gofmt` (or `goimports`)
- **Linting:** Code must pass `golangci-lint` with the project's configuration
- **Idioms:** Follow [Effective Go](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)

## Project Structure

We adhere to a modified Kubebuilder scaffolding that promotes internal encapsulation:

| Directory | Purpose |
|-----------|---------|
| `api/v1alpha1/` | CRD structs and simple validation. **No business logic.** |
| `cmd/` | Binary entry points (controller, provisioner, sentinel) |
| `internal/controller/` | Reconciliation logic, delegates to internal packages |
| `internal/` | Core business logic (infra, config, backup, upgrade) |

The `internal/` convention prevents external imports, allowing us to change APIs freely.

## Naming Conventions

### Casing

- **PascalCase** for exported (public) types, functions, and variables
- **camelCase** for unexported (private) items

### Acronyms

Keep acronyms in consistent case:

```go
// Good
ServeHTTP, userID, parseURL

// Bad
ServeHttp, UserId, ParseUrl
```

### Getters

Do **not** use `Get` prefix for getters:

```go
// Bad
func (c *Cluster) GetName() string

// Good
func (c *Cluster) Name() string
```

### Interfaces

Single-method interfaces should use `-er` suffix:

```go
type Reader interface { Read([]byte) (int, error) }
type CertGenerator interface { Generate(Config) (*tls.Certificate, error) }
```

## Imports

Group imports in this order, separated by blank lines:

1. Standard library
2. Third-party packages
3. Local packages

```go
import (
    "context"
    "fmt"

    "sigs.k8s.io/controller-runtime/pkg/client"

    "github.com/dc-tec/openbao-operator/internal/config"
)
```

## Comments

### GoDoc

Exported functions **must** have GoDoc comments:

```go
// ReconcileTLS ensures TLS certificates exist and are valid.
// It returns true if certificates were rotated.
func ReconcileTLS(ctx context.Context, cluster *v1alpha1.OpenBaoCluster) (bool, error) {
```

### Inline Comments

Use inline comments to explain **why**, not what:

```go
// Use uncached client to avoid race conditions with concurrent updates.
// The cache may be stale if another controller just modified this resource.
if err := r.UncachedClient.Get(ctx, key, secret); err != nil {
```

## Constants

Avoid magic numbers and string literals. Use constants:

```go
const (
    DefaultReplicas    = 3
    TLSSecretSuffix    = "-tls"
    ReconcileInterval  = 30 * time.Second
)
```

For shared constants, use `internal/constants/` package.
