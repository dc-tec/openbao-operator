# Skill: scaffold-e2e-test

Use this skill to add a new E2E test scenario under `test/e2e/`.

## Checklist

1. Keep the scenario focused and tied to a regression surface (ACME, External TLS, Transit unseal, Hardened profile).
2. Prefer creating a dedicated namespace via the e2e framework helper.
3. Assert:
   - CR creation succeeds
   - Core resources exist (ConfigMap, StatefulSet, Services/Routes when relevant)
   - Cluster reaches Ready state (replicas)
4. Keep timeouts realistic but bounded.
5. Ensure cleanup deletes created namespaces/resources.
