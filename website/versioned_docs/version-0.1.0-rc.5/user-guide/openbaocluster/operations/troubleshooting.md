---
slug: /operate/troubleshooting
---

# Troubleshooting

This page covers common failure modes for Hardened and ACME-enabled clusters and how to resolve them.

## Probes fail with x509 (no IP SANs)

<Callout type="failure" title="Symptom">

Pod events show readiness/liveness probe failures like `x509: cannot validate certificate for 127.0.0.1 because it doesn't contain any IP SANs`.

</Callout>

**Cause:**

The serving certificate contains DNS SANs only (common for externally-managed certs), but probes connect to loopback.

**Resolution:**

- Ensure `spec.gateway.hostname` or `spec.ingress.host` is set (or an external Service exists). The OpenBao Operator sets probe SNI (`-servername=...`) based on these fields.
- Ensure your certificate includes the chosen hostname in its SANs.

## ACME domain does not resolve (private ACME CA)

<Callout type="failure" title="Symptom">

- `ConditionDegraded=True` with reason `ACMEDomainNotResolvable`.
- Pod logs show `no such host` or challenge timeouts.

</Callout>

**Cause:**

For private ACME CAs running inside the cluster (for example, an in-cluster PKI), the configured `spec.tls.acme.domains` must resolve via cluster DNS.

**Resolution:**

- Use an internal domain such as `<cluster>-acme.<namespace>.svc` (the OpenBao Operator creates a dedicated `-acme` Service for this use case).
- In local clusters (k3d/k3s), ensure CoreDNS has the required overrides if you use a non-`.svc` domain.

## ACME + HA (Raft) join errors

<Callout type="failure" title="Symptom">

- `certificate signed by unknown authority` during join.
- `certificate is valid for X, not Y` / server name mismatch.

</Callout>

**Resolution:**

- Ensure `spec.tls.acme.domains` contains names that are present in the issued certificate SANs.
- For private ACME CAs, set `spec.configuration.acmeCARoot` and provide a `pki-ca.crt` alongside it in the same mounted volume; the OpenBao Operator uses it for Raft `retry_join` and probe verification.

## Gateway passthrough issues

<Callout type="failure" title="Symptom">

`ConditionDegraded=True` with reason `ACMEGatewayNotConfiguredForPassthrough`.
`GatewayIntegrationReady=False` with reason `GatewayListenerIncompatible`, `GatewayFeatureUnsupported`, or `GatewayNotProgrammed`.

</Callout>

**Resolution:**

- For `tls.mode: ACME`, use `spec.gateway.tlsPassthrough: true` (TLSRoute). TLS termination at the Gateway prevents OpenBao from completing ACME challenges.
- Ensure the referenced Gateway has a `TLS` listener with `tls.mode: Passthrough` (controller support varies).
- Inspect `GatewayIntegrationReady` to confirm the referenced `GatewayClass` is accepted, advertises the required route feature, and the `Gateway` is programmed.

## Public ACME CA cannot reach the endpoint

<Callout type="failure" title="Symptom">

Pod logs show ACME errors such as `Timeout during connect`, `secondary validation`, or repeated `tls-alpn-01` failures against a public CA such as Let's Encrypt.

</Callout>

**Cause:**

The hardened hostname is not publicly reachable on port `443`, or the passthrough edge is only reachable from a restricted source CIDR.

**Resolution:**

- For public ACME, expose the hardened hostname on a dedicated public passthrough listener.
- Do not source-restrict the hardened ACME endpoint to a single client IP.
- Keep restricted admin UIs on a separate terminating edge if needed.

## Kubernetes API egress issues

<Callout type="failure" title="Symptom">

`APIServerNetworkReady=False` with reason `APIServerNetworkConfigurationInvalid`.
`Degraded=True` with reason `APIServerNetworkConfigurationInvalid`.

</Callout>

**Resolution:**

- Set `spec.network.apiServerCIDR` if the in-cluster Kubernetes service VIP cannot be discovered or you want an explicit allow-list.
- If your CNI enforces egress on post-DNAT traffic, also set `spec.network.apiServerEndpointIPs` with the control-plane endpoint IPs.
- If `APIServerNetworkReady=Unknown` with reason `APIServerEndpointIPsRecommended`, the common service-VIP path is configured. Only add `apiServerEndpointIPs` if Kubernetes API connectivity still fails in your environment.

## Hardened profile + AppArmor mismatch

<Callout type="failure" title="Symptom">

`ConditionNodeSecurityCapabilityMismatch=True` with reason `AppArmorUnsupported`.

</Callout>

**Resolution:**

Disable AppArmor in dev clusters that do not support it:

```yaml
spec:
  workloadHardening:
    appArmorEnabled: false
```

## Official OpenBao Documentation

- [TCP Listener Configuration](https://openbao.org/docs/configuration/listener/tcp/)
- [ACME TLS Listener RFC](https://openbao.org/docs/rfcs/acme-tls-listeners/)
- [Operator Raft Command](https://openbao.org/docs/commands/operator/raft/)
