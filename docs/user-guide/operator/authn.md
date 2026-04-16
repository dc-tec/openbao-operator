---
title: Operator Authentication
description: Understand the projected JWT auth path, audience contract, and custom-install checks before you change controller identity wiring.
slug: /get-started/operator-authentication
hide_title: true
pageType: concept
journey: get-started
---

<PageHeader
  title="Operator authentication paths"
  lede="The operator authenticates to OpenBao with a projected Kubernetes ServiceAccount token by default. This page explains the default JWT path, the audience and role-binding requirements behind it, and the checks to run when you customize controller identity wiring."
/>



<DiagramFrame
  title="Default operator auth path"
  caption="Kubernetes issues a projected token for the controller. OpenBao validates that token against the configured JWT auth method and returns a scoped operator token for maintenance work."
  code={`sequenceDiagram
    autonumber
    participant K8s as Kubernetes API
    participant Controller as Operator controller
    participant Bao as OpenBao cluster

    K8s->>Controller: Mount projected token (aud=openbao-internal)
    Controller->>Bao: Login through auth/jwt-operator
    Bao-->>Controller: OpenBao token with openbao-operator policy
    Controller->>Bao: Run health, autopilot, and maintenance requests

    Note over Controller,Bao: Human login is a separate bootstrap path`}
/>

<DecisionTable
  title="Why the JWT path is the default"
  columns={['Property', 'What you get', 'Why it matters']}
  rows={[
    {
      cells: [
        'No stored root token',
        'The controller authenticates with a projected ServiceAccount token instead of a long-lived Secret.',
        'You avoid carrying a static high-privilege credential in Kubernetes just to let the operator do day 2 work.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Automatic rotation',
        'Kubernetes rotates the projected token on its own schedule.',
        'Controller auth ages out naturally instead of depending on manual token rotation discipline.',
      ],
    },
    {
      cells: [
        'Audience binding',
        'The token is scoped to `OPENBAO_JWT_AUDIENCE`, which defaults to `openbao-internal`.',
        'A token issued for OpenBao auth should not be replayable against unrelated services.',
      ],
    },
    {
      cells: [
        'Scoped maintenance policy',
        'The JWT role returns a token with the operator maintenance policy, not blanket admin privileges.',
        'The controller gets the capabilities it needs for health checks, step-down, and autopilot without widening normal access.',
      ],
    },
  ]}
/>

<Callout type="note" title="Operator auth is not human auth">

`spec.selfInit.oidc.enabled: true` bootstraps the controller auth path only.
It does not create a human login method by itself.
If you use self-init, human access should be created in the same bootstrap contract through `selfInit.requests`, not bolted on later as an afterthought.

</Callout>

<DecisionTable
  title="Treat bootstrap auth as two access surfaces"
  columns={['Surface', 'Where it is defined', 'Why it exists']}
  rows={[
    {
      cells: [
        'Operator lifecycle auth',
        '`spec.selfInit.oidc.enabled` or an equivalent manually managed JWT role',
        'Lets the operator perform backup, restore, upgrade, and maintenance work with a short-lived scoped identity.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Human login path',
        '`spec.selfInit.requests` or another deliberate bootstrap path in the same cluster bring-up plan',
        'Ensures someone can actually sign in after the root token is revoked.',
      ],
    },
  ]}
/>

## Self-init and manual bootstrap

<DecisionTable
  title="Choose the bootstrap path"
  columns={['Path', 'Use it when', 'Operator behavior', 'Watch for']}
  rows={[
    {
      cells: [
        'Self-init with OIDC',
        'You want the supported production path and the cluster is allowed to bootstrap its own operator auth surface.',
        'The operator configures JWT auth, discovery, the `openbao-operator` policy, and the bound role automatically.',
        'Bootstrap a human login path in `selfInit.requests` at the same time so the cluster is usable after root-token revocation.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Manual JWT configuration',
        'You are carrying a compatibility workflow, a custom install shape, or a controlled bootstrap sequence.',
        'You create the JWT auth method, policy, and role binding yourself.',
        'Rendered ServiceAccount name, namespace, and audience drift are the usual breakpoints.',
      ],
      emphasis: 'caution',
    },
  ]}
/>

<CommandBlock
  language="hcl"
  label="configure"
  title="The operator policy is intentionally narrow"
  code={`path "sys/health" {
  capabilities = ["read"]
}

path "sys/step-down" {
  capabilities = ["sudo", "update"]
}

path "sys/storage/raft/autopilot/configuration" {
  capabilities = ["read", "update"]
}`}
>
  Keep the controller policy focused on maintenance work. Backup, restore, and upgrade jobs should authenticate through their own roles instead of inheriting the controller scope.
</CommandBlock>

<CommandBlock
  language="bash"
  label="configure"
  title="Manual JWT role example for a custom controller identity"
  code={`bao write auth/jwt-operator/role/openbao-operator \\
  role_type="jwt" \\
  bound_audiences="openbao-internal" \\
  user_claim="sub" \\
  bound_subject="system:serviceaccount:platform-security:demo-openbao-operator-controller" \\
  token_policies="openbao-operator" \\
  token_ttl="1h"`}
>
  Replace the namespace, ServiceAccount name, and audience with the values produced by your rendered install, not the defaults from an example manifest.
</CommandBlock>

## What must stay aligned

<DecisionTable
  kind="reference"
  title="Custom-install checks"
  columns={['Surface', 'Must match', 'What breaks when it drifts']}
  rows={[
    {
      cells: [
        'Controller identity',
        'Rendered controller ServiceAccount name and operator namespace',
        'The OpenBao JWT role or admission-policy identity references the wrong controller subject.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Projected token mount',
        'The controller Deployment still mounts the `openbao-token` projected volume',
        'The controller loses its default path to authenticate to OpenBao at all.',
      ],
    },
    {
      cells: [
        'JWT audience',
        '`OPENBAO_JWT_AUDIENCE`, the projected token audience, and the OpenBao role `bound_audiences`',
        'A valid ServiceAccount token is rejected because it was issued for a different audience contract.',
      ],
    },
    {
      cells: [
        'Discovery access',
        'The controller can reach `/.well-known/openid-configuration` and the JWKS endpoint',
        'OpenBao cannot validate the projected token even though the controller identity itself is correct.',
      ],
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Typical auth failures"
  columns={['Symptom', 'Most likely cause', 'Check first']}
  rows={[
    {
      cells: [
        '`permission denied` when the controller calls OpenBao',
        'JWT audience mismatch or an incorrect bound subject',
        'Rendered identity and `OPENBAO_JWT_AUDIENCE` alignment',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Self-init completes but controller auth never settles',
        'OIDC discovery is unreachable or incomplete',
        'Kubernetes API discovery reachability and cluster condition output',
      ],
    },
    {
      cells: [
        'Backup or restore auth fails even though controller auth works',
        'Executor jobs use separate roles and ServiceAccounts',
        'Operator authorization and the job-specific auth path',
      ],
    },
  ]}
/>

<NextActions
  title="Continue the support path"
  items={[
    {
      label: 'Operator authorization',
      description: 'Review which policies belong to controller, backup, restore, and upgrade work.',
      docId: 'user-guide/operator/authz',
    },
    {
      label: 'Operator identity and access',
      description: 'Trace which Kubernetes identity maps to which OpenBao-side auth boundary.',
      docId: 'user-guide/operator/identity-and-access',
    },
    {
      label: 'Return to installation',
      description: 'Go back to the install page once the rendered identity and audience contract feel mechanical.',
      docId: 'user-guide/operator/installation',
    },
  ]}
/>

## Official OpenBao background

- [JWT/OIDC auth method](https://openbao.org/docs/auth/jwt/)
- [Kubernetes auth method](https://openbao.org/docs/auth/kubernetes/)
- [Token concepts](https://openbao.org/docs/concepts/tokens/)
