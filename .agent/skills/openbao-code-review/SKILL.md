# Skill: openbao-code-review

Use this skill for a focused review of operator changes that affect Hardened/ACME clusters.

## Review Areas

1. **TLS contracts**
   - Certificate SAN requirements match SNI usage (probes, retry_join, Gateway BackendTLSPolicy).
2. **ACME mode correctness**
   - No TLS Secrets are expected/mounted.
   - HA join settings are compatible with issued certificates and CA trust.
3. **Networking**
   - Required Services/Routes are created deterministically.
   - NetworkPolicy examples cover required egress/ingress.
4. **Status**
   - Failures surface as actionable Conditions with stable reasons.
5. **Docs and tests**
   - User guide recipes match current behavior.
   - Unit/golden/e2e coverage exists for new logic.
