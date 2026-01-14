# Conventional Commits

We follow the [Conventional Commits](https://www.conventionalcommits.org/) specification to ensure a consistent commit history and to enable automated changelog generation and semantic versioning.

## Format

Each commit message consists of a **header**, a **body**, and a **footer**.

```text
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

!!! example "Good Commit Message"
    ```text
    feat(backup): add support for S3-compatible storage

    Introduces a new `s3` stanza to the Backup configuration to allow
    streaming snapshots to MinIO and AWS S3.

    Closes #123
    ```

## Commit Types

| Type | Description | SemVer Bump |
| :--- | :--- | :--- |
| `feat` | A new feature | **Minor** |
| `fix` | A bug fix | **Patch** |
| `docs` | Documentation only changes | Patch |
| `style` | Formatting, missing semi-colons, etc; no code change | Patch |
| `refactor` | A code change that neither fixes a bug nor adds a feature | Patch |
| `perf` | A code change that improves performance | Patch |
| `test` | Adding missing tests or correcting existing tests | Patch |
| `build` | Changes that affect the build system or external dependencies | Patch |
| `ci` | Changes to our CI configuration files and scripts | Patch |
| `chore` | Other changes that don't modify src or test files | Patch |
| `revert` | Reverts a previous commit | Patch |

## Scopes

We enforce specific scopes to categorize changes by component.

### Core Components

| Scope | Area |
| :--- | :--- |
| `core` | General core logic, main package |
| `api` | CRD definitions and API types |
| `controller` | Reconciler logic |
| `infra` | Infrastructure management (StatefulSet, Service) |
| `config` | HCL configuration generation |
| `security` | TLS, encryption, security profiles |
| `rbac` | RBAC generation and management |

### Features

| Scope | Area |
| :--- | :--- |
| `backup` | Backup Manager and snapshot logic |
| `restore` | Restore Manager and recovery logic |
| `upgrade` | Upgrade Manager and version transitions |
| `bluegreen` | Blue/Green upgrade strategy |

### Testing

| Scope | Area |
| :--- | :--- |
| `test(unit)` | Unit tests (`_test.go`) |
| `test(integration)` | EnvTest integration tests |
| `test(e2e)` | End-to-End Kind tests |

### Operations & Meta

| Scope | Area |
| :--- | :--- |
| `charts` | Helm chart changes |
| `manifests` | Generated YAML manifests |
| `deps` | Dependency updates (`go.mod`) |
| `ci` | GitHub Actions workflow changes |
| `build` | Makefiles, Dockerfiles |
| `docs` | Documentation changes |
| `ai` | AI instructions and rules |

## Breaking Changes

To indicate a breaking change, append `!` after the type/scope, or include `BREAKING CHANGE:` in the footer.

```text
feat(api)!: remove deprecated v1alpha1 fields
```

OR

```text
feat(api): remove deprecated v1alpha1 fields

BREAKING CHANGE: The `autoUnseal` field has been removed in favor of `unsealConfig`.
```
