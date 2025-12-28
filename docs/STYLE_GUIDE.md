# OpenBao Operator Documentation Style Guide

This guide ensures a consistent voice, structure, and appearance across all OpenBao Operator documentation.

## 1. General Principles

- **Voice:** Professional, concise, and direct. Use the imperative mood for instructions (e.g., "Run the command" instead of "You should run the command").
- **Audience:** DevOps engineers, SREs, and Platform engineers. Assume familiarity with Kubernetes but explain OpenBao-specific concepts.
- **Terminology:**
  - Always use **OpenBao Operator** (capitalized).
  - Use **OpenBao** for the software, not "openbao" (unless referring to the command/package in code).
  - Use **Kubernetes** or **K8s**, not "k8s" (lowercase) in prose.

## 2. Formatting & Markdown

### 2.1 Headers

- Use **Title Case** for all headers.
- **H1 (#)** is reserved for the page title.
- **H2 (##)** for major sections.
- **H3 (###)** for subsections.
- Do **not** use manual numbering (e.g., "1. Introduction"). Let the site structure handle flow.

### 2.2 Code Blocks

Always specify the language for syntax highlighting:

```yaml
apiVersion: v1
kind: Pod
```

```sh
kubectl get pods
```

### 2.3 Admonitions / Callouts

Use standard admonitions to highlight important info:

!!! note
    Useful context that doesn't fit the main flow.

!!! warning
    Potential pitfalls or irreversible actions.

## 3. Diagrams (Mermaid)

Use Mermaid for all diagrams to ensure they are version-controllable and theme-aware.

- **Sequence Diagrams:** Use `autonumber`.
- **Flowcharts:** Use `graph TD` or `graph LR`.

## 4. User Guide Structure

Every User Guide page should follow this template where applicable:

1. **Title:** Noun-based (e.g., "Backup Configuration").
2. **Introduction:** 1-2 sentences explaining what this feature does.
3. **Prerequisites:** Bullet points of what is needed (e.g., "OpenBao v2.0+", "S3 Bucket").
4. **Configuration:** YAML examples and explanation of fields.
5. **Operations/Usage:** How to use, trigger, or monitor the feature.

## 5. File Organization

- `docs/user-guide/`: Task-oriented guides for end-users.
- `docs/reference/`: Technical references, matrices, and API specs.
- `docs/architecture/`: Design docs and internal details.
- `docs/contributing/`: Guides for developers working *on* the operator.
