---
trigger: always_on
glob: "**/*.go"
description: Naming conventions for the OpenBao Operator
---

# Naming Conventions

See [Go Style](docs/contributing/standards/go-style.md).

## Go Naming

- **PascalCase**: Exported types, functions, variables
- **camelCase**: Unexported (private) items
- **Acronyms**: Consistent case (`ServeHTTP`, `userID`, not `ServeHttp`)
- **Getters**: No `Get` prefix (`Name()` not `GetName()`)
- **Interfaces**: Single-method uses `-er` suffix (`Reader`, `Writer`)

## Kubernetes Resource Naming

Resource names MUST be deterministic from the parent CR:

```go
// Good - deterministic
secretName := fmt.Sprintf("%s-tls", cluster.Name)
configName := fmt.Sprintf("%s-config", cluster.Name)

// Bad - random/timestamp
secretName := fmt.Sprintf("%s-%d", cluster.Name, time.Now().Unix())
```

## Constants

- Use `internal/constants/` for shared constants
- Use typed constants, not raw strings:

```go
type Phase string
const PhaseRunning Phase = "Running"
```

## Metrics

- Prefix: `openbao_`
- Required labels: `namespace`, `name`
