# Security Practices

Security best practices for code contributions to the OpenBao Operator.

## File Permissions

When writing files (even in tests), use appropriate permissions:

| Type | Permission | Octal |
|------|------------|-------|
| Secrets, keys | Read/write owner only | `0600` |
| Config files | Read all, write owner | `0644` |
| Directories | Execute/read all | `0755` |

```go
// Writing a secret file
if err := os.WriteFile(path, data, 0600); err != nil {
    return err
}

// Writing a config file
if err := os.WriteFile(path, config, 0644); err != nil {
    return err
}
```

## Cryptography

### Random Number Generation

**Always use `crypto/rand`** for security-sensitive randomness:

```go
import "crypto/rand"

// Good - cryptographically secure
token := make([]byte, 32)
if _, err := rand.Read(token); err != nil {
    return err
}

// NEVER use math/rand for security purposes
import "math/rand"  // DON'T USE FOR TOKENS/KEYS
```

### Certificate Generation

Use the `internal/pki/` package for certificate operations. Do not implement custom crypto logic.

## Input Validation

Validate all inputs from the CRD before use:

```go
// Validate image reference
if !isValidImageRef(cluster.Spec.Image) {
    return fmt.Errorf("invalid image reference: %s", cluster.Spec.Image)
}

// Validate numeric ranges
if cluster.Spec.Replicas < 1 || cluster.Spec.Replicas > 7 {
    return fmt.Errorf("replicas must be between 1 and 7")
}
```

### Path Traversal Prevention

When constructing file paths from user input:

```go
import "path/filepath"

// Clean the path to prevent traversal
safePath := filepath.Clean(filepath.Join(baseDir, userInput))

// Verify it's still under the base directory
if !strings.HasPrefix(safePath, baseDir) {
    return fmt.Errorf("invalid path: escapes base directory")
}
```

## No Shelling Out

Controllers and internal packages **MUST NOT** shell out to external tools:

```go
// Bad - shelling out
cmd := exec.Command("kubectl", "get", "pods")
output, err := cmd.Output()

// Good - use client-go
var pods corev1.PodList
if err := r.Client.List(ctx, &pods, client.InNamespace(ns)); err != nil {
    return err
}
```

This applies to:

- ❌ `kubectl`, `helm`, `bao`, `vault`
- ❌ Cloud CLIs (`aws`, `gcloud`, `az`)
- ❌ Any external process execution

Use:

- ✅ Kubernetes client-go / controller-runtime
- ✅ Official cloud SDKs (AWS SDK, Google Cloud Go, Azure SDK)
- ✅ OpenBao Go client

## Secrets Handling

### Memory Management

Minimize the time secrets are held in memory:

```go
// Clear sensitive data when done
defer func() {
    for i := range unsealKey {
        unsealKey[i] = 0
    }
}()
```

### No Logging

Never log secret values, even partially:

```go
// Bad
log.Info("Secret value", "data", string(secret.Data["key"]))
log.Info("Token prefix", "prefix", token[:10])

// Good - log only metadata
log.Info("Secret loaded", "name", secret.Name, "keys", maps.Keys(secret.Data))
```

## Separation of Concerns

### API Types are "Dumb"

Structs in `api/v1alpha1` must be data containers only:

```go
// Bad - business logic on API type
func (c *OpenBaoCluster) Connect() (*api.Client, error) {
    // DON'T DO THIS
}

// Good - simple helpers only
func (c *OpenBaoCluster) GetCondition(t string) *Condition {
    // This is acceptable
}
```

### No God Objects

Don't pass the entire Reconciler to helper functions:

```go
// Bad
func createTLS(r *Reconciler, cluster *v1alpha1.OpenBaoCluster) error

// Good - pass only what's needed
func createTLS(ctx context.Context, client client.Client, log logr.Logger, cluster *v1alpha1.OpenBaoCluster) error
```
