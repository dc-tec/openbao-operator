# Security Practices

Security is paramount when handling sensitive credentials like Unseal Keys and TLS certificates. Follow these guidelines strictly.

## 1. File Permissions

When creating files, use the most restrictive permissions possible.

| File Type | Octal | Meaning |
| :--- | :--- | :--- |
| **Secrets / Keys** | `0600` | Read/Write by Owner ONLY |
| **Config / Public** | `0644` | Read All, Write Owner |
| **Directories** | `0755` | Execute/Read All, Write Owner |

=== ":material-check: Good Pattern"
    ```go
    // Private Key - 0600
    if err := os.WriteFile(keyPath, keyData, 0600); err != nil {
        return err
    }
    ```

=== ":material-close: Bad Pattern"
    ```go
    // Too permissive!
    os.WriteFile(keyPath, keyData, 0777)
    ```

## 2. Cryptography

### Randomness

**Always** use `crypto/rand` for security-sensitive operations (tokens, keys, passwords).

=== ":material-check: Good Pattern"
    ```go
    import "crypto/rand"

    token := make([]byte, 32)
    if _, err := rand.Read(token); err != nil {
        return err
    }
    ```

=== ":material-close: Bad Pattern"
    ```go
    import "math/rand"

    // Not cryptographically secure!
    token := rand.Int63()
    ```

### Certificates

Do not implement custom certificate logic. Use the `internal/pki` package, which defaults to safe algorithms (ECDSA P-256 or RSA 2048+).

## 3. No Shelling Out

!!! failure "Forbidden"
    Controllers and internal packages **MUST NOT** execute external binaries (`kubectl`, `helm`, `bao`, `vault`).

    Shelling out introduces injection vulnerabilities, dependency requirements, and performance overhead.

=== ":material-check: Good Pattern"
    ```go
    // Use the Go client
    var pods corev1.PodList
    if err := r.Client.List(ctx, &pods, client.InNamespace(ns)); err != nil {
        return err
    }
    ```

=== ":material-close: Bad Pattern"
    ```go
    // Shell Injection Vulnerability!
    cmd := exec.Command("kubectl", "get", "pods", "-n", ns)
    ```

## 4. Input Validation

Validate **all** user input from Custom Resources before using it.

=== "Path Traversal"
    ```go
    // Clean and Verify
    cleanPath := filepath.Clean(filepath.Join(baseDir, userInput))
    if !strings.HasPrefix(cleanPath, baseDir) {
        return fmt.Errorf("invalid path: %s", userInput)
    }
    ```

=== "Numeric Ranges"
    ```go
    if spec.Replicas < 1 || spec.Replicas > 9 {
        return fmt.Errorf("replicas must be between 1 and 9")
    }
    ```

## 5. Secrets Handling

### No Logging

!!! danger "Do Not Log Secrets"
    **NEVER** log the content of secrets, tokens, or unseal keys.
    Be careful with `fmt.Sprintf("%v", obj)`, which might print struct fields.

=== ":material-check: Good Pattern"
    ```go
    log.Info("Secret loaded", "name", secret.Name, "len", len(secret.Data))
    ```

=== ":material-close: Bad Pattern"
    ```go
    log.Info("Got secret", "data", secret.Data)
    ```

### Memory Scrubbing

Minimize the exposure window of sensitive data in memory.

```go
func handleKeys(keys []byte) {
    // Zero out memory when done
    defer func() {
        for i := range keys {
            keys[i] = 0
        }
    }()

    // ... use keys ...
}
```
