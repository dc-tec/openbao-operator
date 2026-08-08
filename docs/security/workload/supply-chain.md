---
title: Supply-Chain Verification
hide_title: true
pageType: concept
journey: security
description: Signature verification, transparency-log checks, digest pinning, and release-artifact validation for operator-managed OpenBao images.
---

<PageHeader
  title="Supply-chain verification"
  lede="Signature verification, digest pinning, and separate trust roots for the main OpenBao image and helper images such as init, backup, restore, and upgrade executors."
/>



<DiagramFrame
  title="Verification flow before reconcile"
  caption="The controller resolves a tag, verifies the signature and optional transparency-log evidence, then writes only the verified digest into managed workloads."
  code={`flowchart LR
    Registry["Container registry"] --> Resolve["Resolve tag to digest"]
    Resolve --> Verify["Verify signature"]
    Verify --> Rekor["Check Rekor transparency log"]
    Rekor --> Pin["Pin digest in workload spec"]
    Pin --> Reconcile["Reconcile StatefulSet or Job"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Registry,Rekor read;
    class Resolve,Verify process;
    class Pin,Reconcile write;`}
/>

<DecisionTable
  title="Supply-chain controls at a glance"
  columns={["Control", "What it blocks", "Operational note"]}
  rows={[
    {
      cells: [
        "Signature verification",
        "Unsigned or improperly signed images.",
        "Trust material can come from a public key or from keyless identity matching.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Transparency-log verification",
        "Opaque signing events with no public evidence trail.",
        "This is on by default and is usually only disabled for disconnected environments.",
      ],
    },
    {
      cells: [
        "Digest pinning",
        "Tag-mutation races between verification time and pull time.",
        "The controller writes the immutable digest so the kubelet pulls the exact artifact that was verified.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Admission digest enforcement",
        "Direct API writes that bypass the controller and reintroduce mutable tags.",
        "This is defense in depth, not a replacement for reconciliation-time verification.",
      ],
    },
  ]}
/>

## Trust surfaces

<DecisionTable
  kind="reference"
  title="What gets verified"
  columns={["Image surface", "Configuration field", "Why it is separate"]}
  rows={[
    {
      cells: [
        "Main OpenBao server image",
        "`spec.imageVerification`",
        "The application image can have a different signer and release cadence from the operator helper images.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Init, backup, restore, upgrade, and BlueGreen validation-hook images",
        "`spec.operatorImageVerification`",
        "Helper and custom hook images need a trust policy separate from the OpenBao server image.",
      ],
    },
    {
      cells: [
        "Helm chart and release artifacts",
        "Out-of-cluster verification",
        "These should be verified during installation and upgrade planning even though the controller does not reconcile them in-cluster.",
      ],
    },
  ]}
/>

<DecisionTable
  kind="reference"
  title="Trust-source choices"
  columns={["Path", "Use it when", "Watch for"]}
  rows={[
    {
      cells: [
        "Keyless identity matching",
        "You consume official project images and want issuer-plus-subject verification without maintaining your own public key distribution.",
        "Custom registries and forks should set explicit trust material rather than relying on official-image defaults.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Public-key verification",
        "Your organization signs images with its own key or needs an offline verification path.",
        "Rotate the public key deliberately and make sure the key distribution path is auditable.",
      ],
    },
    {
      cells: [
        "Warning-only rollout",
        "You are onboarding verification in a non-production environment and need visibility before enforcing block behavior.",
        "This should be a transition state, not the final production posture.",
      ],
      emphasis: "caution",
    },
  ]}
/>

## Failure behavior

<DecisionTable
  title="Verification failure policy"
  columns={["Policy", "Controller behavior", "When to use it"]}
  rows={[
    {
      cells: [
        "Block",
        "Stops reconciliation for the managed workload and records the failure in status.",
        "Use this for production. If you do not trust the artifact, do not roll it out.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Warn",
        "Records and logs the verification failure but continues with the original image reference.",
        "Use this only while you are standing up signing infrastructure or testing policy adoption.",
      ],
      emphasis: "caution",
    },
  ]}
/>

<Callout type="warning" title="Hardened profile expectations">

The Hardened profile is opinionated here. The intent is that image verification stays on, mutable-tag rollouts are not the norm, and official-image defaults are only used when the operator can still tie trust back to the expected signer identity.

</Callout>

<Callout type="note" title="Custom Hardened trust roots are delegated">

In the `Hardened` profile, official image-verification defaults can be used without extra RBAC. If a CR author sets custom trust material such as `publicKey`, issuer or subject matchers, regexp matchers, or `ignoreTlog`, that identity also needs the `useimagetrustroots` verb on the target `OpenBaoCluster`.

</Callout>

## Verify published release artifacts

<CommandBlock
  language="yaml"
  label="configure"
  title="Enable verification for both workload image surfaces"
  code={`spec:
  imageVerification:
    enabled: true
    failurePolicy: Block
    imagePullSecrets:
      - name: cluster-registry-creds
  operatorImageVerification:
    enabled: true
    failurePolicy: Block
    imagePullSecrets:
      - name: cluster-registry-creds`}
>
  Treat the main OpenBao image and operator helper images as separate trust surfaces. They may share policy, but they should not share assumptions blindly.
</CommandBlock>

<Callout type="note" title="Private-registry verification needs controller Secret read">

Verification pull Secrets are read by the controller, not by the kubelet. In multi-tenant mode, the provisioner keeps that access name-scoped by granting `get` only on the tenant Secrets referenced from enabled `spec.imageVerification.imagePullSecrets` and `spec.operatorImageVerification.imagePullSecrets`.

</Callout>

<CommandBlock
  language="bash"
  label="verify"
  title="Verify a published operator image outside the cluster"
  code={`IMAGE="ghcr.io/dc-tec/openbao-operator@sha256:<digest>"

cosign verify \\
  --new-bundle-format=true \\
  --certificate-identity "https://github.com/dc-tec/openbao-operator/.github/workflows/release.yml@refs/tags/X.Y.Z" \\
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \\
  "\${IMAGE}"`}
>
  Use the matching tag or release metadata for the version you are about to deploy, and verify by digest rather than by mutable tag.
</CommandBlock>

<NextActions
  title="Continue workload protections"
  items={[
    {
      label: "TLS and identity",
      description: "Connect artifact trust back to the workload trust path that presents the service to clients.",
      docId: "security/workload/tls",
    },
    {
      label: "Production posture",
      description: "See how the Hardened profile treats verification and warning-only modes.",
      docId: "security/fundamentals/profiles",
    },
    {
      label: "Air-gapped and private registries",
      description: "Switch to the configuration guide when you need the operational path for disconnected environments.",
      docId: "user-guide/openbaocluster/configuration/air-gapped",
    },
  ]}
/>
