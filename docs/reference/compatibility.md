# Compatibility Matrix

This document defines the supported Kubernetes and OpenBao versions for the OpenBao Operator.

!!! info "Support Policy"
    We aim to support the **latest 3 minor versions** of Kubernetes and the **latest major** version of OpenBao.

## 1. Kubernetes Versions

We run full E2E tests against these versions.

| Version | Status | Notes |
| :--- | :--- | :--- |
| **v1.35** | <span style="color: #22c55e">**Supported**</span> | Primary CI target |
| **v1.34** | <span style="color: #22c55e">**Supported**</span> | Verified in Nightly |
| **v1.33** | <span style="color: #22c55e">**Supported**</span> | Minimum supported version |
| **v1.32** | <span style="color: #ef4444">**End of Life**</span> | No longer tested |

## 2. OpenBao Versions

| Version | Status | Notes |
| :--- | :--- | :--- |
| **2.4.x** | <span style="color: #22c55e">**Supported**</span> | Primary target |
| **2.3.x** | <span style="color: #f59e0b">**Deprecated**</span> | Best effort support |

## 3. CI Validation Matrix

We treat our CI configuration as the source of truth for compatibility.

| Workflow | Scope | Versions Tested |
| :--- | :--- | :--- |
| **PR Gate** | Logic & Config | K8s 1.35 + OpenBao 2.4.0 |
| **Nightly** | Full Coverage | K8s 1.33, 1.34, 1.35 + OpenBao 2.4.x |

!!! warning "Production Upgrade"
    Always validate new Kubernetes or OpenBao versions in a staging environment before upgrading production, even if they are listed as "Supported".
