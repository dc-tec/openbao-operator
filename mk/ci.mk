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
ci-core: security-ci security-scan-built-images lint-ci verify-fmt verify-tidy verify-go-toolchain-sync verify-workflows verify-vendor verify-generated verify-api-stability-inventory report-crd-compatibility verify-e2e-manifest test-ci fuzz verify-openbao-config-compat docs-build verify-helm helm-test ## Run all CI checks except E2E tests (cluster-independent).

CI_MANAGER_SCAN_IMG ?= local/openbao-operator:ci
CI_INIT_SCAN_IMG ?= local/openbao-init:ci
CI_BACKUP_SCAN_IMG ?= local/openbao-backup:ci
CI_UPGRADE_SCAN_IMG ?= local/openbao-upgrade:ci

.PHONY: security-scan-built-images
security-scan-built-images: ## Build the manager and helper images locally and run the CI-equivalent Trivy image scans.
	@$(MAKE) docker-build IMG='$(CI_MANAGER_SCAN_IMG)'
	@$(MAKE) docker-build-init IMG='$(CI_INIT_SCAN_IMG)'
	@$(MAKE) docker-build-backup IMG='$(CI_BACKUP_SCAN_IMG)'
	@$(MAKE) docker-build-upgrade IMG='$(CI_UPGRADE_SCAN_IMG)'
	@$(MAKE) security-scan-image IMG='$(CI_MANAGER_SCAN_IMG)'
	@$(MAKE) security-scan-image IMG='$(CI_INIT_SCAN_IMG)'
	@$(MAKE) security-scan-image IMG='$(CI_BACKUP_SCAN_IMG)'
	@$(MAKE) security-scan-image IMG='$(CI_UPGRADE_SCAN_IMG)'

.PHONY: security-scan-built-manager
security-scan-built-manager: security-scan-built-images ## Backward-compatible alias for the CI-equivalent built-image Trivy scans.

.PHONY: pentest-smoke
pentest-smoke: ## Run "pentest" labeled e2e tests against an existing cluster (requires E2E_OPERATOR_IMAGE).
	@$(MAKE) test-e2e-existing E2E_LABEL_FILTER='pentest && security'
