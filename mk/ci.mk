##@ CI

# CI_INCLUDE_E2E controls whether E2E tests are run as part of the ci target.
# Set to "true" to include E2E tests (requires Kind cluster and takes ~15+ minutes).
# Example: make ci CI_INCLUDE_E2E=true
CI_INCLUDE_E2E ?= false

.PHONY: ci
ci: ## Run full CI pipeline locally (fail-fast). Set CI_INCLUDE_E2E=true to include E2E tests.
ifeq ($(CI_INCLUDE_E2E),true)
	$(MAKE) ci-core test-e2e-ci
else
	$(MAKE) ci-core
endif
	@echo "✅ All CI checks passed!"

.PHONY: ci-core
ci-core: security-ci lint-ci verify-fmt verify-tidy verify-vendor verify-generated test-ci fuzz verify-openbao-config-compat docs-build verify-helm helm-test ## Run all CI checks except E2E tests (cluster-independent).

.PHONY: pentest-smoke
pentest-smoke: ## Run "pentest" labeled e2e tests against an existing cluster (requires E2E_OPERATOR_IMAGE).
	@$(MAKE) test-e2e-existing E2E_LABEL_FILTER='pentest && security'
