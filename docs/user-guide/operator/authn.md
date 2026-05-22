---
title: Operator Authentication
description: Projected JWT auth path, audience contract, and custom-install checks for controller identity wiring.
slug: /get-started/operator-authentication
hide_title: true
pageType: concept
journey: get-started
---

<PageHeader
  title="Operator authentication paths"
  lede="Default JWT authentication, the audience contract behind it, and the checks to run when you customize controller identity wiring."
/>



<DiagramFrame
  title="Default operator auth path"
  caption="Kubernetes issues a projected token for the controller. The operator sends that JWT with each OpenBao maintenance request using inline authentication."
  code={`sequenceDiagram
    autonumber
    participant K8s as Kubernetes API
    participant Controller as Operator controller
    participant Bao as OpenBao cluster

    K8s->>Controller: Mount projected token (aud=openbao-internal)
    Controller->>Bao: Maintenance request with inline auth headers
    Bao->>Bao: Validate auth/jwt-operator role
    Bao-->>Controller: Maintenance response

    Note over Controller,Bao: Human login is a separate bootstrap path`}
/>

<DecisionTable
  title="JWT default path"
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
        'The JWT role grants the operator maintenance policy to each authenticated request, not blanket admin privileges.',
        'The controller gets the capabilities it needs for health checks, step-down, and autopilot without widening normal access.',
      ],
    },
  ]}
/>

<Callout type="note" title="Inline authentication is the default">

For supported OpenBao versions, JWT-backed operator requests use OpenBao inline authentication by default.
The controller, backup jobs, restore jobs, and upgrade jobs still use the same projected ServiceAccount JWTs,
role names, audiences, and policies. The transport changes from a separate login request plus `X-Vault-Token`
to inline auth headers on the actual OpenBao request.

</Callout>

<DecisionTable
  kind="reference"
  title="JWT transport strategy"
  columns={['Strategy', 'How it authenticates', 'When to use it']}
  rows={[
    {
      cells: [
        '`inline`',
        'Sends the projected ServiceAccount JWT through OpenBao inline auth headers on each operator-owned request.',
        'Default for supported OpenBao versions. Use this for normal operation.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        '`standard`',
        'Performs the legacy JWT login request and then sends the returned OpenBao token as `X-Vault-Token`.',
        'Temporary compatibility switch when an intermediary drops custom inline auth headers or enforces header-size limits.',
      ],
      emphasis: 'caution',
    },
  ]}
/>

<CommandBlock
  language="bash"
  label="configure"
  title="Use the standard JWT fallback"
  code={`kubectl -n openbao-operator-system set env deployment/openbao-operator-controller-manager \\
  OPENBAO_JWT_AUTH_STRATEGY=standard`}
>
  Leave the variable unset, or set it to `inline`, for the default inline-auth path. The operator propagates this setting to backup, restore, and upgrade executor jobs.
</CommandBlock>

<Callout type="note" title="Controller and human authentication are separate">

`spec.selfInit.oidc.enabled: true` bootstraps the controller auth path only.
It does not create a human login method by itself.
If you use self-init, create human access in the same bootstrap contract through `selfInit.requests` so the cluster has an operator path and a human path from the start.

</Callout>

<Callout type="warning" title="Bootstrap does not mean ongoing policy reconciliation">

`spec.selfInit.oidc.enabled: true` bootstraps the controller JWT auth method, policy, and role.
After bootstrap, the operator uses that auth surface but does not continue mutating OpenBao
policies on its own. When a later operator release needs an additional controller capability,
the human operator must update the `openbao-operator` policy explicitly.

</Callout>

<DecisionTable
  title="Bootstrap authentication surfaces"
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

<DecisionTable
  kind="reference"
  title="Who owns policy changes after bootstrap"
  columns={['Install shape', 'Who creates the initial policy', 'Who updates it later']}
  rows={[
    {
      cells: [
        'Self-init with OIDC',
        'The bootstrap contract created by the operator during self-init.',
        'The human operator. Self-init does not keep mutating OpenBao policies after the cluster is initialized.',
      ],
      emphasis: 'recommended',
    },
    {
      cells: [
        'Manual JWT configuration',
        'The cluster administrator.',
        'The cluster administrator.',
      ],
    },
  ]}
/>

<CommandBlock
  language="hcl"
  label="configure"
  title="Controller policy scope"
  code={`path "sys/health" {
  capabilities = ["read"]
}

path "sys/step-down" {
  capabilities = ["sudo", "update"]
}

path "sys/storage/raft/configuration" {
  capabilities = ["read"]
}

path "sys/storage/raft/remove-peer" {
  capabilities = ["update"]
}

path "sys/storage/raft/autopilot/configuration" {
  capabilities = ["read", "update"]
}

path "sys/storage/raft/autopilot/state" {
  capabilities = ["read"]
}`}
>
  Keep the controller policy focused on maintenance work. Backup, restore, and upgrade jobs authenticate through their own roles instead of inheriting the controller scope.
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
  Replace the namespace, ServiceAccount name, and audience with the values from your rendered install, not the example defaults.
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
