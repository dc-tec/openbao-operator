# Security Practices

Security is paramount when handling sensitive credentials like Unseal Keys and TLS certificates. Follow these guidelines strictly.

## 1. File Permissions

When creating files, use the most restrictive permissions possible.

| File Type | Octal | Meaning |
| :--- | :--- | :--- |
| **Secrets / Keys** | `0600` | Read/Write by Owner ONLY |
| **Config / Public** | `0644` | Read All, Write Owner |
| **Directories** | `0755` | Execute/Read All, Write Owner |

<Tabs groupId="good-pattern-bad-pattern">

<TabItem value="good-pattern" label="Good Pattern">

```go
// Private Key - 0600
if err := os.WriteFile(keyPath, keyData, 0600); err != nil {
    return err
}
```

</TabItem>

<TabItem value="bad-pattern" label="Bad Pattern">

```go
// Too permissive!
os.WriteFile(keyPath, keyData, 0777)
```

</TabItem>

</Tabs>

## 2. Cryptography

### Randomness

**Always** use `crypto/rand` for security-sensitive operations (tokens, keys, passwords).

<Tabs groupId="good-pattern-bad-pattern">

<TabItem value="good-pattern" label="Good Pattern">

```go
import "crypto/rand"

token := make([]byte, 32)
if _, err := rand.Read(token); err != nil {
    return err
}
```

</TabItem>

<TabItem value="bad-pattern" label="Bad Pattern">

```go
import "math/rand"

// Not cryptographically secure!
token := rand.Int63()
```

</TabItem>

</Tabs>

### Certificates

Do not implement custom certificate logic. Use the `internal/service/certs` package and shared certificate helpers.

## 3. No Shelling Out

<Callout type="failure" title="Forbidden">

Controllers and internal packages **MUST NOT** execute external binaries (`kubectl`, `helm`, `bao`, `vault`).

Shelling out introduces injection vulnerabilities, dependency requirements, and performance overhead.

</Callout>

<Tabs groupId="good-pattern-bad-pattern">

<TabItem value="good-pattern" label="Good Pattern">

```go
// Use the Go client
var pods corev1.PodList
if err := r.Client.List(ctx, &pods, client.InNamespace(ns)); err != nil {
    return err
}
```

</TabItem>

<TabItem value="bad-pattern" label="Bad Pattern">

```go
// Shell Injection Vulnerability!
cmd := exec.Command("kubectl", "get", "pods", "-n", ns)
```

</TabItem>

</Tabs>

## 4. Input Validation

Validate **all** user input from Custom Resources before using it.

<Tabs groupId="path-traversal-numeric-ranges">

<TabItem value="path-traversal" label="Path Traversal">

```go
// Clean and Verify
cleanPath := filepath.Clean(filepath.Join(baseDir, userInput))
if !strings.HasPrefix(cleanPath, baseDir) {
    return fmt.Errorf("invalid path: %s", userInput)
}
```

</TabItem>

<TabItem value="numeric-ranges" label="Numeric Ranges">

```go
if spec.Replicas < 1 || spec.Replicas > 9 {
    return fmt.Errorf("replicas must be between 1 and 9")
}
```

</TabItem>

</Tabs>

## 5. Secrets Handling

### No Logging

<Callout type="danger" title="Do Not Log Secrets">

**NEVER** log the content of secrets, tokens, or unseal keys.
Be careful with `fmt.Sprintf("%v", obj)`, which might print struct fields.

</Callout>

<Tabs groupId="good-pattern-bad-pattern">

<TabItem value="good-pattern" label="Good Pattern">

```go
log.Info("Secret loaded", "name", secret.Name, "len", len(secret.Data))
```

</TabItem>

<TabItem value="bad-pattern" label="Bad Pattern">

```go
log.Info("Got secret", "data", secret.Data)
```

</TabItem>

</Tabs>

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

