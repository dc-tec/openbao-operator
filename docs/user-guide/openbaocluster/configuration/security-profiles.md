---
title: Security Profiles
slug: /configure/security-profiles
hide_title: true
pageType: task
journey: configure
description: Choose the cluster posture first, including bootstrap, unseal, TLS, and image-verification expectations for development versus Hardened production.
---

<PageHeader
  title="Choose the operating posture before you tune anything else."
  lede="`spec.profile` is the top-level decision that shapes bootstrap, unseal, TLS, image verification, and failure tolerance. Pick the posture you plan to keep operating, then let the rest of the cluster baseline follow from that choice."
/>



<DecisionTable
  title="Choose the profile deliberately"
  columns={["Profile", "Use it when", "What it assumes", "Avoid it when"]}
  rows={[
    {
      cells: [
        "Hardened",
        "The cluster is intended to become a real production service.",
        "External unseal, self-initialization, verified TLS, and supply-chain guardrails are part of the normal path.",
        "You cannot meet the external trust, identity, or networking requirements yet.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Development",
        "You need a fast local or evaluation path and accept weaker security defaults temporarily.",
        "Bootstrap material may live in Kubernetes Secrets, TLS can be operator-managed, and verification can be relaxed.",
        "You are trying to define the baseline that will survive into production.",
      ],
      emphasis: "caution",
    },
  ]}
/>

<DiagramFrame
  title="How the profile shapes the baseline"
  caption="The profile is not cosmetic. It determines whether the cluster can rely on operator-generated trust material and stored bootstrap credentials or whether those paths are explicitly disallowed."
  code={`flowchart LR
    Profile["spec.profile"] --> Bootstrap["Bootstrap path"]
    Profile --> Unseal["Unseal root of trust"]
    Profile --> TLS["TLS source"]
    Profile --> Verify["Image verification"]
    Profile --> Risk["Status and admission guardrails"]

    Bootstrap --> SelfInit["Self-initialization or standard init"]
    Unseal --> KMS["External KMS / transit / static"]
    TLS --> External["External / ACME / operator-managed"]
    Verify --> Policy["Block or warn"]
    Risk --> Conditions["Conditions and validation policy"]

    classDef read fill:transparent,stroke:#79c0ab,stroke-width:2px,color:#e6f4ef;
    classDef process fill:transparent,stroke:#fdd0a4,stroke-width:2px,color:#e6f4ef;
    classDef write fill:transparent,stroke:#87d6be,stroke-width:2px,color:#e6f4ef;

    class Profile process;
    class Bootstrap,Unseal,TLS,Verify,Risk read;
    class SelfInit,KMS,External,Policy,Conditions write;`}
/>

## What actually changes with the profile

<DecisionTable
  kind="reference"
  title="Profile effects"
  columns={["Surface", "Development", "Hardened"]}
  rows={[
    {
      cells: [
        "Bootstrap credential handling",
        "Manual init is allowed and can leave a root token in a Secret when self-init is disabled.",
        "Self-initialization is the supported path and root-token persistence is not the normal operating model.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Unseal",
        "Static unseal in a Secret is allowed for fast evaluation.",
        "Use an external trust source such as cloud KMS or transit. Treat static unseal as a non-production exception.",
      ],
    },
    {
      cells: [
        "TLS",
        "Operator-managed TLS is acceptable for development and internal evaluation.",
        "Use `External` or `ACME`; the certificate authority should not be operator-generated in production.",
      ],
    },
    {
      cells: [
        "Image verification",
        "Can be introduced gradually and warning-only rollouts are possible.",
        "Verification is expected, warning-only behavior is not the production posture, and official-image defaults still verify when trust material is omitted.",
      ],
    },
    {
      cells: [
        "Networking and jobs",
        "You can tolerate more permissive local defaults while standing up the cluster.",
        "Backup and other lifecycle paths should assume explicit egress and identity wiring before go-live.",
      ],
    },
  ]}
/>

## Representative starting points

<Tabs groupId="configure-profile-hardened-development">
  <TabItem value="hardened" label="Hardened">

<CommandBlock
  language="yaml"
  label="configure"
  title="Start from the supported production baseline"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: prod-cluster
spec:
  profile: Hardened
  replicas: 3
  version: "2.4.4"
  selfInit:
    enabled: true
  tls:
    enabled: true
    mode: External
  unseal:
    type: awskms
    awskms:
      region: us-east-1
      kmsKeyID: alias/openbao-unseal
  imageVerification:
    enabled: true
    failurePolicy: Block
  operatorImageVerification:
    enabled: true
    failurePolicy: Block`}
>
  Hardened is the supported production path. It assumes an external trust source for unseal, non-operator-managed TLS, and a self-initializing bootstrap flow.
</CommandBlock>

  </TabItem>
  <TabItem value="development" label="Development">

<CommandBlock
  language="yaml"
  label="configure"
  title="Use the lightest safe evaluation baseline"
  code={`apiVersion: openbao.org/v1alpha1
kind: OpenBaoCluster
metadata:
  name: dev-cluster
spec:
  profile: Development
  version: "2.4.4"
  replicas: 1
  tls:
    enabled: true
  unseal:
    type: static`}
>
  Development is for local testing, proof-of-concept work, and environments where you explicitly accept stored bootstrap material and weaker trust boundaries.
</CommandBlock>

  </TabItem>
</Tabs>

<Callout type="warning" title="Do not let development defaults drift into production">

The dangerous part of the Development profile is not that it exists; it is that it feels easy to keep. If the cluster matters, switch to Hardened before other systems begin to depend on it.

</Callout>

## Choose the unseal root of trust

<DecisionTable
  title="Unseal options by posture"
  columns={["Path", "Use it when", "Why it fits or does not fit"]}
  rows={[
    {
      cells: [
        "Cloud KMS",
        "You run in AWS, GCP, Azure, or another managed platform with a usable external key service.",
        "This is usually the cleanest Hardened path because the root of trust stays outside Kubernetes.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Transit",
        "You already run a central OpenBao cluster or equivalent external trust service.",
        "This works well for multi-cluster or hybrid environments where central trust management is intentional.",
      ],
    },
    {
      cells: [
        "PKCS#11 or KMIP",
        "You need HSM-backed or enterprise key-management integration.",
        "Valid for production, but usually more specialized and operationally heavier than cloud KMS or transit.",
      ],
    },
    {
      cells: [
        "Static Secret",
        "You need a local development path and understand the blast radius.",
        "This is convenient but keeps decryption material inside the same cluster state you are trying to protect.",
      ],
      emphasis: "caution",
    },
  ]}
/>

<Tabs groupId="configure-unseal-common-patterns">
  <TabItem value="aws-kms" label="AWS KMS">

<CommandBlock
  language="yaml"
  label="configure"
  title="Use AWS KMS for Hardened unseal"
  code={`spec:
  profile: Hardened
  serviceAccount:
    annotations:
      eks.amazonaws.com/role-arn: "arn:aws:iam::123456789012:role/openbao-awskms"
  unseal:
    type: awskms
    awskms:
      kmsKeyID: "arn:aws:kms:us-east-1:123456789012:key/..."
      region: "us-east-1"`}
/>

  </TabItem>
  <TabItem value="gcp-kms" label="GCP KMS">

<CommandBlock
  language="yaml"
  label="configure"
  title="Use GCP Cloud KMS for Hardened unseal"
  code={`spec:
  profile: Hardened
  serviceAccount:
    annotations:
      iam.gke.io/gcp-service-account: "openbao@my-project.iam.gserviceaccount.com"
  unseal:
    type: gcpckms
    gcpCloudKMS:
      project: "my-project"
      region: "us-central1"
      keyRing: "openbao-ring"
      cryptoKey: "openbao-key"`}
/>

  </TabItem>
  <TabItem value="azure-kv" label="Azure Key Vault">

<CommandBlock
  language="yaml"
  label="configure"
  title="Use Azure Key Vault for Hardened unseal"
  code={`spec:
  profile: Hardened
  serviceAccount:
    annotations:
      azure.workload.identity/client-id: "87654321-4321-4321-4321-210987654321"
  podMetadata:
    labels:
      azure.workload.identity/use: "true"
  unseal:
    type: azurekeyvault
    azureKeyVault:
      vaultName: "my-vault"
      keyName: "openbao-key"`}
/>

  </TabItem>
  <TabItem value="transit" label="Transit">

<CommandBlock
  language="yaml"
  label="configure"
  title="Use a central OpenBao transit key for unseal"
  code={`spec:
  profile: Hardened
  unseal:
    type: transit
    credentialsSecretRef:
      name: transit-unseal-creds
    transit:
      address: "https://central-openbao.example.com"
      keyName: "tenant-1-key"
      mountPath: "transit"`}
>
  The referenced Secret should hold the transit token and any optional CA or client-certificate material required by that upstream cluster.
</CommandBlock>

  </TabItem>
</Tabs>

## Optional runtime hardening

<CommandBlock
  language="yaml"
  label="configure"
  title="Enable AppArmor when the nodes support it"
  code={`spec:
  workloadHardening:
    appArmorEnabled: true`}
>
  AppArmor is opt-in because support depends on the underlying node OS and cluster runtime. Pair this with the broader workload baseline in <SiteLink docId="security/workload/workload-security">Pod and runtime security</SiteLink>.
</CommandBlock>

<NextActions
  title="Continue cluster baseline"
  items={[
    {
      label: "Unseal configuration",
      description: "Use the provider-by-provider Secret and mounted-file contract page once you know which root of trust you want.",
      docId: "user-guide/openbaocluster/configuration/unseal",
    },
    {
      label: "Self-initialization",
      description: "Configure the bootstrap requests and operator OIDC flow that follow from the profile choice.",
      docId: "user-guide/openbaocluster/configuration/self-init",
    },
    {
      label: "External access",
      description: "Choose the TLS and exposure pattern that matches the baseline you just picked.",
      docId: "user-guide/openbaocluster/configuration/external-access",
    },
    {
      label: "Workload protections",
      description: "Review the runtime and supply-chain controls expected behind the Hardened posture.",
      docId: "security/workload/index",
    },
  ]}
/>
