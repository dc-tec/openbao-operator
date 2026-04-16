---
title: Security Practices
description: Secure coding practices for OpenBao Operator contributors, including file permissions, randomness, input validation, secret handling, and controller safety boundaries.
pageType: concept
journey: contribute
---

<PageHeader
  title="Secure coding practices"
  lede="OpenBao Operator handles sensitive material and security-relevant control paths. This page covers least-privilege permissions, standard cryptographic primitives, controller-safe execution, and explicit care around secrets in memory and logs."
/>

<DecisionTable
  title="Secure coding defaults"
  columns={["Area", "Expected default", "Avoid"]}
  rows={[
    {
      cells: [
        "Filesystem permissions",
        "Use the narrowest permissions possible, especially `0600` for keys and secrets.",
        "World-writable or broadly readable secret material.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Randomness",
        "Use `crypto/rand` for security-sensitive values.",
        "`math/rand` for tokens, passwords, keys, or nonces.",
      ],
    },
    {
      cells: [
        "Controller execution model",
        "Use Go libraries and Kubernetes clients directly.",
        "Shelling out to `kubectl`, `helm`, `bao`, `vault`, or similar binaries from controllers.",
      ],
      emphasis: "caution",
    },
    {
      cells: [
        "Input validation",
        "Validate paths, ranges, and other CR-driven input before use.",
        "Passing raw user input into filesystem or runtime-sensitive operations.",
      ],
    },
    {
      cells: [
        "Secret handling",
        "Do not log secret contents and minimize their lifetime in memory.",
        "Debug output that prints token, key, or secret payload fields.",
      ],
    },
  ]}
/>

## Minimal secure examples

```go
// private key file permissions
if err := os.WriteFile(keyPath, keyData, 0o600); err != nil {
    return err
}

// cryptographically secure randomness
token := make([]byte, 32)
if _, err := rand.Read(token); err != nil {
    return err
}
```

<Callout type="warning" title="Controllers must not shell out">

Shelling out introduces injection risk, hidden runtime dependencies, and slower, harder-to-test control paths. Use Kubernetes clients and internal helpers instead.

</Callout>

## Input and secret handling

- Clean and validate filesystem paths before use.
- Enforce numeric and enum bounds from CR input explicitly.
- Never log secret payloads, even in debug-only paths.
- Zero sensitive byte slices after use when the code keeps them in memory for any meaningful period.

<NextActions
  title="Related secure-contributor guides"
  items={[
    {
      label: "Supply chain security",
      description: "Open the artifact-trust controls when the change moves from secure coding into build, provenance, or release security.",
      to: "/contribute/supply-chain-security",
    },
    {
      label: "Dependency license policy",
      description: "Use the dependency policy when the security question is whether a new dependency is even shippable.",
      to: "/contribute/dependency-licenses",
    },
    {
      label: "Security docs",
      description: "Return to the operator-facing security section when you need the runtime trust model rather than contributor implementation rules.",
      to: "/docs/security",
    },
  ]}
/>
