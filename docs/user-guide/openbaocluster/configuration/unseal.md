---
title: Unseal Configuration
slug: /configure/unseal
hide_title: true
pageType: task
journey: configure
description: Choose the unseal root of trust deliberately and use the exact Secret and mounted-file contract the operator validates for each unseal mode.
---

<PageHeader
  title="Treat unseal as a trust contract, not just a field to satisfy."
  lede="The unseal path decides where the root of trust lives, how credentials reach the Pods, and which Secret keys or mounted files the operator expects. Use this page when you want the concrete contract, not just the high-level posture guidance."
/>



<Callout type="important" title="Hardened posture prefers external trust">

For production-oriented clusters, use an external trust source such as cloud KMS, transit, KMIP, OCI KMS, or PKCS#11. Static unseal remains useful for development and controlled exceptions, but it keeps decryption material inside Kubernetes.

</Callout>

<DecisionTable
  title="Choose the unseal path deliberately"
  columns={["Path", "Use it when", "Main operator expectation"]}
  rows={[
    {
      cells: [
        "Static",
        "You need the lightest development or evaluation path.",
        "The operator manages a per-cluster Secret unless you deliberately override that path.",
      ],
      emphasis: "caution",
    },
    {
      cells: [
        "Transit",
        "You already run a central OpenBao trust root or another intentional upstream unseal service.",
        "The credentials Secret must carry the transit token and any referenced TLS files.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "Cloud KMS",
        "You want the root of trust outside Kubernetes and your platform already provides a KMS path.",
        "Prefer workload identity or ambient credentials; only use Secrets when that identity path is unavailable or intentionally overridden.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "KMIP / PKCS#11",
        "You need HSM-backed or enterprise key-management integration.",
        "The Secret contract is mostly file- or PIN-oriented and must match the rendered configuration exactly.",
      ],
    },
  ]}
/>

## General rules

<Callout type="note" title="What the operator validates before Pods can use Secret-backed credentials">

- `spec.unseal.credentialsSecretRef` must reference a Secret in the same namespace as the `OpenBaoCluster`.
- Any unseal field that points to a mounted credential file must use a path under `/etc/bao/seal-creds`.
- The Secret key name must match the filename used in the mounted path.
- When you use private ACME trust rooted under `/etc/bao/seal-creds`, include `pki-ca.crt` in the same Secret so probes and day-2 operations can trust the ACME issuer too.

</Callout>

<CommandBlock
  language="yaml"
  label="reference"
  title="How mounted credential paths map to Secret keys"
  code={`spec:
  unseal:
    credentialsSecretRef:
      name: unseal-creds
    transit:
      tlsCACert: "/etc/bao/seal-creds/ca.crt"
      tlsClientCert: "/etc/bao/seal-creds/client.crt"
      tlsClientKey: "/etc/bao/seal-creds/client.key"
    gcpCloudKMS:
      credentials: "/etc/bao/seal-creds/credentials.json"
    kmip:
      clientCert: "/etc/bao/seal-creds/kmip-client.crt"
      clientKey: "/etc/bao/seal-creds/kmip-client.key"
      caCert: "/etc/bao/seal-creds/kmip-ca.crt"`}
>
  In this example, the Secret named `unseal-creds` must contain keys named `ca.crt`, `client.crt`, `client.key`, `credentials.json`, `kmip-client.crt`, `kmip-client.key`, and `kmip-ca.crt`.
</CommandBlock>

## Provider credential contracts

<DecisionTable
  kind="reference"
  title="Secret requirements by provider"
  columns={["Provider", "When a Secret is needed", "Required keys or file contract", "Notes"]}
  rows={[
    {
      cells: [
        "Static",
        "Usually not needed because the operator generates the Secret automatically.",
        "Generated Secret `<cluster-name>-unseal-key` with key `key`.",
        "Use this only for development or controlled exceptions. If you replace it manually, keep the same key name.",
      ],
    },
    {
      cells: [
        "Transit",
        "Needed whenever you do not rely only on an inline token and Secret-backed files are referenced.",
        "`token`, plus any mounted files referenced by `tlsCACert`, `tlsClientCert`, and `tlsClientKey`.",
        "If client cert auth is used, the certificate and key must both be present and form a valid key pair.",
      ],
      emphasis: "recommended",
    },
    {
      cells: [
        "AWS KMS",
        "Needed only when you are not using IRSA, ambient credentials, or another default AWS credential chain.",
        "`AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`.",
        "Use Secrets only when workload identity is not the intended path.",
      ],
    },
    {
      cells: [
        "GCP Cloud KMS",
        "Needed only when `spec.unseal.gcpCloudKMS.credentials` points at a mounted file instead of using Workload Identity or ADC.",
        "A Secret key matching the configured file name, usually `credentials.json`, containing valid JSON credentials.",
        "The path must live under `/etc/bao/seal-creds`.",
      ],
    },
    {
      cells: [
        "Azure Key Vault",
        "Needed only when you are not using Managed Identity or Azure Workload Identity.",
        "`AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, and `AZURE_CLIENT_SECRET`.",
        "Managed identity is the cleaner Hardened path when your platform supports it.",
      ],
    },
    {
      cells: [
        "OCI KMS",
        "Needed only when `spec.unseal.ocikms.authTypeAPIKey=true`.",
        "`config`, plus the Secret key referenced by `key_file` inside the OCI SDK config.",
        "The OCI config must define `user`, `fingerprint`, `tenancy`, `region`, and `key_file` in profile `[DEFAULT]`, and `key_file` must point under `/etc/bao/seal-creds`.",
      ],
    },
    {
      cells: [
        "KMIP",
        "Needed whenever client cert, key, or CA files are sourced from mounted credentials.",
        "Secret keys matching the filenames referenced by `clientCert`, `clientKey`, and optional `caCert` under `/etc/bao/seal-creds`.",
        "The client certificate and key must form a valid pair; the CA bundle must be valid PEM when set.",
      ],
    },
    {
      cells: [
        "PKCS#11",
        "Needed when `spec.unseal.pkcs11.pin` is omitted.",
        "`BAO_HSM_PIN`.",
        "The operator also requires either `slot` or `tokenLabel`, but not both.",
      ],
    },
  ]}
/>

## Static unseal details

<CommandBlock
  language="bash"
  label="apply"
  title="Create or replace the static unseal Secret manually"
  code={`kubectl -n <namespace> create secret generic <cluster-name>-unseal-key \\
  --from-literal=key='<UNSEAL_KEY>' \\
  --dry-run=client -o yaml | kubectl apply -f -`}
>
  The operator-generated static Secret uses the name `<cluster-name>-unseal-key` and the data key `key`.
</CommandBlock>

<Callout type="warning" title="Private ACME trust piggybacks on the unseal credentials Secret">

If `spec.configuration.acmeCARoot` points under `/etc/bao/seal-creds`, the Secret referenced by `spec.unseal.credentialsSecretRef` must also contain `pki-ca.crt`. This is how the operator and helper clients trust a private ACME issuer during probes and day-2 operations.

</Callout>

<NextActions
  title="Continue baseline setup"
  items={[
    {
      label: "Security profiles",
      description: "Return to the posture guide when you need to decide whether this unseal path fits Development or Hardened operation.",
      docId: "user-guide/openbaocluster/configuration/security-profiles",
    },
    {
      label: "Self-initialization",
      description: "Configure bootstrap and operator OIDC once the trust root and unseal path are chosen.",
      docId: "user-guide/openbaocluster/configuration/self-init",
    },
    {
      label: "Recover a sealed cluster",
      description: "Use the recovery runbook when the unseal contract is already failing in a live cluster.",
      docId: "user-guide/openbaocluster/recovery/sealed-cluster",
    },
  ]}
/>
