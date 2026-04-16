---
title: Production Checklist
description: Verify security, durability, observability, and readiness before you route real traffic through an OpenBao cluster.
slug: /operate/production-checklist
hide_title: true
pageType: task
journey: operate
---

<PageHeader
  title="Production readiness checklist"
  lede="Use this checklist after the first cluster path succeeds and before teams depend on the service. It covers security posture, data protection, observability, and the operator-reported signals that should be clean before production use."
/>

<DecisionTable
  title="Production gates"
  columns={['Gate', 'What must be true', 'Why it matters', 'Go deeper']}
  rows={[
    {
      cells: [
        'Security posture',
        'The cluster runs the hardened path: secure profile, external seal, deliberate TLS, and self-init with real auth methods.',
        'The defaults that help evaluation can become long-lived risk in production.',
        'Security profiles, self-init, and workload TLS configuration.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Durability',
        'Storage, replica count, and scheduled backups are all deliberate and already tested.',
        'Upgrades, restore workflows, and voter recovery all assume the data path is stable.',
        'Backups, storage, and topology spread.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Observability',
        'Metrics, logs, and alerts reach the systems operators actually watch.',
        'Incidents are slower and riskier when the first debugging session starts after go-live.',
        'Observability and network egress configuration.',
      ],
    },
    {
      cells: [
        'Cluster readiness',
        'The status conditions show healthy convergence and no unresolved integration blockers.',
        'A production launch should start from a stable status surface, not an optimistic assumption.',
        'Use the final verification commands on this page.',
      ],
    },
  ]}
/>

## Lock down the security baseline

- Set `spec.profile: Hardened` so the workload starts from the strict controller posture rather than the evaluation defaults.
- Use a non-static external seal such as Transit, cloud KMS, `ocikms`, `kmip`, or `pkcs11`. Do not keep long-lived unseal keys in Kubernetes Secrets for the production path.
- Confirm your Kubernetes cluster already encrypts Secrets at rest. The operator cannot compensate for an unencrypted control plane.
- Use `ACME` or `External` TLS for public or shared edges. Avoid `OperatorManaged` certificates for public-facing production entry points.
- Enable `spec.selfInit` and configure real user authentication in `spec.selfInit.requests` so the first operator-driven bootstrap does not end in a lockout.
- If you rely on operator lifecycle auth for backups and upgrades, enable `spec.selfInit.oidc.enabled: true` or deliberately provision the equivalent JWT roles yourself.

<Callout type="warning" title="Do not stop at install success">

A cluster that initializes successfully still needs security hardening, backup readiness, and clean status conditions before it is ready for production.

</Callout>

## Enforce the tenant guardrails

- Verify the ValidatingAdmissionPolicies and related guardrails are installed and enforced, including:
  - `openbao-validate-openbaocluster`
  - `openbao-validate-openbao-tenant`
  - `openbao-validate-openbaorestore`
  - `openbao-lock-controller-statefulset-mutations`
  - `openbao-lock-managed-resource-mutations`
  - `openbao-enforce-managed-image-digests`
  - `openbao-restrict-provisioner-rbac`
  - `openbao-restrict-provisioner-namespace-mutations`
  - `openbao-restrict-provisioner-tenant-governance`
  - `openbao-restrict-controller-rbac`
  - `openbao-restrict-controller-secret-writes`
- Confirm that the operator namespace, tenant onboarding flow, and shared-controller trust boundaries match the tenancy model you chose during `Get Started`.

<CommandBlock
  language="bash"
  label="inspect"
  title="Inspect the control-plane baseline"
  code={`kubectl get validatingadmissionpolicy | grep openbao
kubectl get deploy -n <operator-namespace>
kubectl get openbaotenant -A`}
>
  The exact number of policies and controller Deployments depends on the features you enabled, but the OpenBao guardrail set should be visible before you bring real tenants onto the platform.
</CommandBlock>

## Make the cluster durable

- Set explicit CPU and memory `requests` and `limits`. A cluster that only works under zero pressure is not production-ready.
- Choose a low-latency StorageClass and set `spec.storage.storageClassName` explicitly for new clusters. The effective storage class is not something you want to discover by accident after PVC creation.
- Use at least three replicas for a highly available Raft cluster and verify the Kubernetes nodes span the intended zones or failure domains.
- Configure scheduled backups and test a restore path before the first risky upgrade.
- Confirm `spec.network.egressRules` allow the cluster to reach the services it really depends on: cloud KMS, OIDC discovery, backup storage, and any external gateway edges.

<Callout type="note" title="Backups are part of the production gate">

Include backup success and restore confidence in the launch checklist rather than deferring them to later follow-up work.

</Callout>

## Prove observability and operational response

- Configure metrics scraping through Prometheus Operator (`ServiceMonitor`) or VictoriaMetrics Operator (`VMServiceScrape`).
- Grant the scraping identity permission to read `/metrics` and keep TLS verification strict in production.
- Make sure structured logs including `cluster_name` and `cluster_namespace` reach the log system your operators actually use.
- Alert on backup staleness, degradation, reconciliation failures, and other conditions that should wake a human before tenants feel the failure.

## Verify the cluster before routing traffic

<CommandBlock
  language="bash"
  label="verify"
  title="Inspect the final readiness surface"
  code={`kubectl describe openbaocluster <name> -n <namespace>
kubectl get openbaocluster <name> -n <namespace> -o jsonpath='{.status.phase}{"\\n"}{range .status.conditions[*]}{.type}={.status}{"\\n"}{end}'`}
>
  Run both commands from the target namespace so you can see the reconciler status, recent events, and the final condition set in one pass.
</CommandBlock>

<DecisionTable
  kind="reference"
  title="Signals to see before go-live"
  columns={['Signal', 'Healthy state', 'Why it is important']}
  rows={[
    {
      cells: [
        'Phase',
        'Running',
        'The cluster has converged past bootstrap and is not stuck in an intermediate lifecycle state.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Available',
        'True',
        'The workload is up and the operator believes the service is available to consumers.',
      ],
    },
    {
      cells: [
        'ProductionReady',
        'True',
        'This is the clearest signal that the cluster passed the production-readiness gate.',
      ],
    },
    {
      cells: [
        'Integration-specific conditions',
        'Healthy for the features you enabled, such as CloudUnsealIdentityReady, GatewayIntegrationReady, APIServerNetworkReady, or BackupConfigurationReady.',
        'These conditions expose dependency problems that may not show up as plain pod readiness failures.',
      ],
    },
  ]}
/>

<NextActions
  title="Continue operating"
  items={[
    {
      label: 'Configure backups',
      description: 'Lock in the snapshot path before you plan the first upgrade or maintenance window.',
      docId: 'user-guide/openbaocluster/operations/backups',
    },
    {
      label: 'Plan upgrades',
      description: 'Choose the right rollout strategy before you patch the version field on a production cluster.',
      docId: 'user-guide/openbaocluster/operations/upgrades',
    },
    {
      label: 'Troubleshoot the cluster',
      description: 'Use the incident routing guide when health, TLS, gateway, or network assumptions drift after launch.',
      docId: 'user-guide/openbaocluster/operations/troubleshooting',
    },
  ]}
/>
