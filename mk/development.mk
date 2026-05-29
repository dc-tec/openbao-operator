##@ Development

.PHONY: bootstrap
bootstrap: controller-gen kustomize crd-ref-docs envtest setup-envtest golangci-lint ginkgo govulncheck go-licenses gomu gotestsum dlv air benchstat semgrep ## Install repo-managed tools and local development dependencies.
	@if command -v "$(NPM)" >/dev/null 2>&1; then \
		$(MAKE) ast-grep; \
	else \
		echo "Skipping ast-grep bootstrap because npm is not available."; \
	fi
	@if command -v "$(DOCS_NPM)" >/dev/null 2>&1; then \
		$(MAKE) docs-deps; \
	else \
		echo "Skipping docs bootstrap because npm is not available."; \
	fi
	@$(MAKE) git-hooks-install
	@echo "Bootstrap complete."
	@echo "Run 'make doctor' to validate external prerequisites."

.PHONY: git-hooks-install
git-hooks-install: ## Configure the repo-local Git hooks path.
	@chmod +x .githooks/pre-commit .githooks/pre-push hack/dev/pre-commit.sh hack/dev/pre-push.sh
	@git config --local core.hooksPath .githooks
	@echo "Configured repo-local Git hooks: .githooks (pre-commit, pre-push)"

.PHONY: git-hooks-uninstall
git-hooks-uninstall: ## Remove the repo-local Git hooks path.
	@current="$$(git config --local --get core.hooksPath || true)"; \
	if [ "$$current" = ".githooks" ]; then \
		git config --local --unset core.hooksPath; \
		echo "Removed repo-local Git hooks configuration."; \
	else \
		echo "Repo-local Git hooks were not configured."; \
	fi

.PHONY: doctor
doctor: ## Validate local prerequisites for the main contributor workflow.
	@bash hack/dev/doctor.sh

.PHONY: tilt-up
tilt-up: ## Launch the local Kubernetes dev loop in Tilt. Use TILT_ARGS for extra flags.
	@command -v "$(TILT)" >/dev/null 2>&1 || { \
		echo "tilt is required. Install it from https://docs.tilt.dev/install.html"; \
		exit 1; \
	}
	"$(TILT)" up $(TILT_ARGS)

.PHONY: tilt-down
tilt-down: ## Tear down Tilt-managed resources in the current kube context.
	@command -v "$(TILT)" >/dev/null 2>&1 || { \
		echo "tilt is required. Install it from https://docs.tilt.dev/install.html"; \
		exit 1; \
	}
	"$(TILT)" down $(TILT_ARGS)

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	# Generate CRDs and webhooks (shared across both controllers)
	"$(CONTROLLER_GEN)" crd webhook paths="./api/..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	"$(CONTROLLER_GEN)" object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: verify-fmt
verify-fmt: ## Verify all Go code is gofmt'd (does not modify files).
	@unformatted="$$(find . -path ./vendor -prune -o -name '*.go' -print0 | xargs -0 gofmt -l)"; \
	if [ -n "$$unformatted" ]; then \
		echo "The following files are not gofmt'd:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

.PHONY: vet
vet: ## Run go vet against code.
	GOFLAGS="$(GOFLAGS_VENDOR)" go vet ./...

.PHONY: test
test: manifests generate fmt vet ## Run unit tests (fast, no envtest).
	GOFLAGS="$(GOFLAGS_VENDOR)" go test $$(GOFLAGS="$(GOFLAGS_VENDOR)" go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: test-sum
test-sum: manifests generate fmt vet gotestsum ## Run unit tests with gotestsum output and JUnit XML.
	@mkdir -p "$(TEST_ARTIFACT_DIR)"
	GOFLAGS="$(GOFLAGS_VENDOR)" "$(GOTESTSUM)" --format="$(GOTESTSUM_FORMAT)" --junitfile "$(TEST_ARTIFACT_DIR)/unit.xml" -- -coverprofile "$(TEST_ARTIFACT_DIR)/unit.cover.out" $$(GOFLAGS="$(GOFLAGS_VENDOR)" go list ./... | grep -v /e2e)

.PHONY: test-ci
test-ci: manifests generate vet setup-envtest ## Run unit + integration tests in CI mode (does not modify tracked files).
	KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" GOFLAGS="$(GOFLAGS_VENDOR)" go test $$(GOFLAGS="$(GOFLAGS_VENDOR)" go list ./... | grep -v /e2e) -tags=integration -coverprofile cover.out

.PHONY: test-integration
test-integration: manifests generate vet setup-envtest ## Run envtest-based integration tests (envtest; requires -tags=integration).
	KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" GOFLAGS="$(GOFLAGS_VENDOR)" go test $$(GOFLAGS="$(GOFLAGS_VENDOR)" go list ./... | grep -v /e2e) -tags=integration -count=1 -v

.PHONY: test-integration-sum
test-integration-sum: manifests generate vet setup-envtest gotestsum ## Run envtest-based integration tests with gotestsum output and JUnit XML.
	@mkdir -p "$(TEST_ARTIFACT_DIR)"
	KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" GOFLAGS="$(GOFLAGS_VENDOR)" "$(GOTESTSUM)" --format="$(GOTESTSUM_FORMAT)" --junitfile "$(TEST_ARTIFACT_DIR)/integration.xml" -- -tags=integration -count=1 -coverprofile "$(TEST_ARTIFACT_DIR)/integration.cover.out" $$(GOFLAGS="$(GOFLAGS_VENDOR)" go list ./... | grep -v /e2e)

.PHONY: fuzz
fuzz: ## Run the curated fuzz smoke sweep across repo fuzz targets.
	@FUZZTIME="$(FUZZTIME)" FUZZ_GOMAXPROCS="$(FUZZ_GOMAXPROCS)" FUZZ_TARGET_FILTER="$(FUZZ_TARGET_FILTER)" GOFLAGS="$(GOFLAGS_VENDOR)" bash hack/ci/fuzz.sh

.PHONY: fuzz-long
fuzz-long: FUZZTIME=20s
fuzz-long: ## Run the longer fuzz sweep used by nightly CI.
	@FUZZTIME="$(FUZZTIME)" FUZZ_GOMAXPROCS="$(FUZZ_GOMAXPROCS)" FUZZ_TARGET_FILTER="$(FUZZ_TARGET_FILTER)" GOFLAGS="$(GOFLAGS_VENDOR)" bash hack/ci/fuzz.sh

.PHONY: verify-tidy
verify-tidy: ## Verify go.mod/go.sum are tidy (does not modify tracked files).
	@GOFLAGS="-mod=mod" go mod tidy
	@{ \
		git diff --exit-code -- go.mod go.sum; \
	} || { \
		echo "go.mod/go.sum are not tidy. Run 'go mod tidy' and commit the result."; \
		git --no-pager diff -- go.mod go.sum; \
		exit 1; \
	}

.PHONY: verify-vendor
verify-vendor: ## Verify vendor/ is synchronized with go.mod/go.sum.
	@GOFLAGS="-mod=mod" go mod vendor
	@{ \
		git diff --exit-code -- vendor; \
	} || { \
		echo "vendor/ is out of date. Run 'go mod vendor' and commit the result."; \
		git --no-pager diff -- vendor; \
		exit 1; \
	}

.PHONY: verify-generated
verify-generated: manifests generate api-reference ## Verify generated artifacts are up-to-date (does not modify tracked files).
	@{ \
		git diff --exit-code -- api/v1alpha1 config/crd/bases docs/reference/api.md; \
	} || { \
		echo "Generated artifacts are out of date. Run 'make manifests generate api-reference' and commit the result."; \
		git --no-pager diff -- api/v1alpha1 config/crd/bases docs/reference/api.md; \
		exit 1; \
	}

.PHONY: api-reference
api-reference: crd-ref-docs ## Generate CRD API reference docs from api/v1alpha1.
	@CRD_REF_DOCS_BIN="$(CRD_REF_DOCS)" bash hack/docs/generate-api-reference.sh

.PHONY: verify-openbao-config-compat
verify-openbao-config-compat: ## Validate generated HCL fixtures against upstream OpenBao config parser (semantic).
	@bash hack/ci/openbao-config-compat.sh 2.4.0 2.4.4

.PHONY: report-openbao-config-schema-drift
report-openbao-config-schema-drift: ## Report upstream OpenBao config schema drift across the supported range (non-failing).
	@REPORT_SCHEMA_DRIFT=true bash hack/ci/openbao-config-compat.sh 2.4.0 2.4.4

.PHONY: report-openbao-operator-schema-drift
report-openbao-operator-schema-drift: ## Report operator-vs-upstream OpenBao config schema drift (non-failing).
	@GOFLAGS="$(GOFLAGS_VENDOR)" go run ./hack/tools/openbao_operator_schema_drift --openbao-image-tag 2.4.4

.PHONY: report-ast
report-ast: generate-ast-rules ast-grep ## Run ast-grep rules in report mode (non-failing; warnings only).
	@"$(AST_GREP)" scan -c .ast-grep/sgconfig.yml --report-style=medium .

.PHONY: lint-ast
lint-ast: generate-ast-rules ast-grep ## Run ast-grep rules in strict mode (treat all findings as errors).
	@"$(AST_GREP)" scan -c .ast-grep/sgconfig.yml --report-style=medium --error .

.PHONY: lint-testonly-exports
lint-testonly-exports: ## Verify production exports are not referenced only from tests.
	@GOFLAGS="$(GOFLAGS_VENDOR)" go run ./hack/tools/testonly_exports

.PHONY: test-ast
test-ast: generate-ast-rules ast-grep ## Run ast-grep rule tests.
	@"$(AST_GREP)" test -c .ast-grep/sgconfig.yml

.PHONY: generate-ast-rules
generate-ast-rules: ## Generate ast-grep architecture boundary rules from policy.
	@GOFLAGS="$(GOFLAGS_VENDOR)" go run ./hack/tools/ast_rulegen \
		-policy .ast-grep/policy/architecture-boundaries.yml \
		-out-dir .ast-grep/rules/generated/architecture-boundary

.PHONY: verify-ast-rules-generated
verify-ast-rules-generated: generate-ast-rules ## Verify generated ast-grep rules are up-to-date.
	@status="$$(git status --porcelain -- .ast-grep/rules/generated/architecture-boundary)"; \
	if [ -n "$$status" ]; then \
		echo "Generated ast-grep rules are out of date. Run 'make generate-ast-rules' and commit the result."; \
		echo "$$status"; \
		exit 1; \
	fi

.PHONY: verify-arch-policy
verify-arch-policy: verify-ast-rules-generated ## Verify architecture policy generation integrity.

.PHONY: report-internal-deps
report-internal-deps: ## Generate internal runtime dependency graph/report locally (report-only; non-failing).
	@bash hack/architecture/report-internal-deps.sh

.PHONY: report-internal-deps-snapshot
report-internal-deps-snapshot: report-internal-deps ## Save current internal dependency report as local baseline snapshot.
	@cp dist/architecture/internal-dependency-report.md dist/architecture/internal-dependency-report.baseline.md
	@echo "Saved baseline snapshot:"
	@echo "  dist/architecture/internal-dependency-report.baseline.md"

.PHONY: report-internal-deps-diff
report-internal-deps-diff: ## Compare internal dependency report against local baseline snapshot (report-only; non-failing).
	@bash hack/architecture/diff-internal-deps.sh \
		"$${BASELINE_REPORT:-dist/architecture/internal-dependency-report.baseline.md}" \
		"$${CURRENT_REPORT:-dist/architecture/internal-dependency-report.md}"

.PHONY: helm-sync
helm-sync: manifests ## Sync Helm chart from config/ (CRDs, admission policies).
	@GOFLAGS="$(GOFLAGS_VENDOR)" go run ./hack/helmchart

.PHONY: verify-helm-values
verify-helm-values: ## Verify Helm values.yaml and values.schema.json stay in sync with templates.
	@GOFLAGS="$(GOFLAGS_VENDOR)" go run ./hack/helmvalues

.PHONY: verify-helm
verify-helm: helm-sync verify-helm-values ## Verify Helm chart is up-to-date (does not modify tracked files).
	@{ \
		git diff --exit-code -- charts/openbao-operator/crds charts/openbao-operator/templates/admission charts/openbao-operator/templates/rbac; \
	} || { \
		echo "Helm chart is out of date. Run 'make helm-sync' and commit the result."; \
		git --no-pager diff -- charts/openbao-operator/crds charts/openbao-operator/templates/admission charts/openbao-operator/templates/rbac; \
		exit 1; \
	}

.PHONY: helm-lint
helm-lint: ## Lint the Helm chart.
	@helm lint charts/openbao-operator

.PHONY: helm-test
helm-test: helm-sync helm-lint ## Test the Helm chart without requiring a live cluster.
	@echo "Testing Helm chart: templating with default values..."
	@helm template openbao-operator charts/openbao-operator \
		--namespace openbao-operator-system \
		--include-crds > /dev/null
	@echo "Testing Helm chart: templating with multi-tenant mode..."
	@helm template openbao-operator charts/openbao-operator \
		--namespace openbao-operator-system \
		--include-crds \
		--set tenancy.mode=multi > /dev/null
	@echo "Testing Helm chart: provisioner admission identity follows fullnameOverride..."
	@render="$$(helm template baoctl charts/openbao-operator \
		--namespace openbao-system \
		--include-crds \
		--set tenancy.mode=multi \
		--set fullnameOverride=baoctl)"; \
		echo "$$render" | grep -q "name: baoctl-provisioner"; \
		echo "$$render" | grep -q "'baoctl-provisioner'"; \
		if echo "$$render" | grep -q "'openbao-operator-provisioner'"; then \
			echo "Helm admission policies rendered a stale provisioner ServiceAccount name"; \
			exit 1; \
		fi
	@if grep -R "'openbao-operator-provisioner'" charts/openbao-operator/templates/admission; then \
		echo "Helm admission templates must derive the provisioner ServiceAccount name from helpers"; \
		exit 1; \
	fi
	@echo "Testing Helm chart: templating with single-tenant mode..."
	@helm template openbao-operator charts/openbao-operator \
		--namespace openbao-operator-system \
		--include-crds \
		--set tenancy.mode=single \
		--set tenancy.targetNamespace=openbao-system > /dev/null
	@echo "Testing Helm chart: client-side install render with default values..."
	@helm template openbao-operator charts/openbao-operator \
		--namespace openbao-operator-system \
		--include-crds > /dev/null
	@echo "Helm chart tests passed successfully!"

.PHONY: helm-e2e-smoke
helm-e2e-smoke: ## Helm chart smoke test against a Kind cluster (installs chart and waits for deployments).
	@bash hack/ci/helm-e2e-smoke.sh

.PHONY: helm-template
helm-template: helm-sync ## Template the Helm chart with default values (useful for debugging).
	@helm template openbao-operator charts/openbao-operator \
		--namespace openbao-operator-system \
		--include-crds

.PHONY: helm-package
helm-package: helm-sync ## Package the Helm chart to verify it's valid.
	@mkdir -p dist
	@helm package charts/openbao-operator -d dist
	@echo "Helm chart packaged successfully in dist/"

.PHONY: helm-install
helm-install: helm-sync ## Install the Helm chart from local charts directory. Use IMG=image:tag or IMG=image@digest to override the operator image.
	@if [ -z "$(IMG)" ]; then \
		echo "Warning: IMG not set. Using default image from values.yaml"; \
		helm install openbao-operator charts/openbao-operator \
			--namespace openbao-operator-system \
			--create-namespace; \
	else \
		if echo "$(IMG)" | grep -q '@'; then \
			image_repo=$$(echo "$(IMG)" | cut -d@ -f1); \
			image_digest=$$(echo "$(IMG)" | cut -d@ -f2); \
			echo "Installing with image: $(IMG) (using digest)"; \
			helm install openbao-operator charts/openbao-operator \
				--namespace openbao-operator-system \
				--create-namespace \
				--set image.repository=$$image_repo \
				--set image.digest=$$image_digest; \
		else \
			image_repo=$$(echo "$(IMG)" | sed 's/:[^:]*$$//'); \
			image_tag=$$(echo "$(IMG)" | sed 's/.*://'); \
			echo "Installing with image: $(IMG) (using tag)"; \
			helm install openbao-operator charts/openbao-operator \
				--namespace openbao-operator-system \
				--create-namespace \
				--set image.repository=$$image_repo \
				--set image.tag=$$image_tag; \
		fi; \
	fi

.PHONY: helm-upgrade
helm-upgrade: helm-sync ## Upgrade the Helm chart from local charts directory. Use IMG=image:tag or IMG=image@digest to override the operator image.
	@if [ -z "$(IMG)" ]; then \
		echo "Warning: IMG not set. Using default image from values.yaml"; \
		helm upgrade openbao-operator charts/openbao-operator \
			--namespace openbao-operator-system; \
	else \
		if echo "$(IMG)" | grep -q '@'; then \
			image_repo=$$(echo "$(IMG)" | cut -d@ -f1); \
			image_digest=$$(echo "$(IMG)" | cut -d@ -f2); \
			echo "Upgrading with image: $(IMG) (using digest)"; \
			helm upgrade openbao-operator charts/openbao-operator \
				--namespace openbao-operator-system \
				--set image.repository=$$image_repo \
				--set image.digest=$$image_digest; \
		else \
			image_repo=$$(echo "$(IMG)" | sed 's/:[^:]*$$//'); \
			image_tag=$$(echo "$(IMG)" | sed 's/.*://'); \
			echo "Upgrading with image: $(IMG) (using tag)"; \
			helm upgrade openbao-operator charts/openbao-operator \
				--namespace openbao-operator-system \
				--set image.repository=$$image_repo \
				--set image.tag=$$image_tag; \
		fi; \
	fi

.PHONY: helm-uninstall
helm-uninstall: ## Uninstall the Helm chart from the cluster.
	@helm uninstall openbao-operator --namespace openbao-operator-system || true

.PHONY: test-update-golden
test-update-golden: ## Update golden files for HCL generation tests. Run this when modifying internal/adapter/config/builder.go or related config generation logic.
	UPDATE_GOLDEN=true go test ./internal/adapter/config/... -v

.PHONY: verify-trusted-root
verify-trusted-root: ## Verify that trusted_root.json exists and is valid JSON.
	@if [ ! -f internal/adapter/security/trusted_root.json ]; then \
		echo "Error: trusted_root.json not found. Run 'make update-trusted-root' to download it."; \
		exit 1; \
	fi
	@python3 -m json.tool internal/adapter/security/trusted_root.json > /dev/null 2>&1 || { \
		echo "Error: trusted_root.json is not valid JSON. Run 'make update-trusted-root' to fix it."; \
		exit 1; \
	}
	@echo "trusted_root.json is valid"

DOCS_DIR ?= website
DOCS_NPM ?= $(NPM)
DOCS_VERSION ?=
TEST_ARTIFACT_DIR ?= dist/test
GOTESTSUM_FORMAT ?= pkgname
FUZZTIME ?= 3s
FUZZ_GOMAXPROCS ?= 4
FUZZ_TARGET_FILTER ?=

.PHONY: docs-deps
docs-deps: ## Install Docusaurus site dependencies from lockfile.
	@$(DOCS_NPM) --prefix "$(DOCS_DIR)" ci

.PHONY: docs-build
docs-build: docs-deps ## Build the Docusaurus docs site locally. Writes ./website/build/.
	@$(DOCS_NPM) --prefix "$(DOCS_DIR)" run build

.PHONY: docs-serve
docs-serve: docs-deps ## Serve docs locally. http://localhost:8000
	@$(DOCS_NPM) --prefix "$(DOCS_DIR)" run start

.PHONY: docs-preview
docs-preview: docs-build ## Preview the built docs locally with production behavior, including search. http://localhost:3000
	@$(DOCS_NPM) --prefix "$(DOCS_DIR)" run serve -- --host 0.0.0.0

.PHONY: docs-version
docs-version: docs-deps ## Snapshot the current docs into a versioned Docusaurus release. Set DOCS_VERSION=<version>.
	@test -n "$(DOCS_VERSION)" || { echo "DOCS_VERSION is required, for example: make docs-version DOCS_VERSION=1.2.3"; exit 1; }
	@$(DOCS_NPM) --prefix "$(DOCS_DIR)" run version:docs -- "$(DOCS_VERSION)"

.PHONY: docs-refresh-version
docs-refresh-version: docs-deps ## Refresh an existing release-line docs snapshot from the checked-out docs. Set DOCS_VERSION=<X.Y.0>.
	@test -n "$(DOCS_VERSION)" || { echo "DOCS_VERSION is required, for example: make docs-refresh-version DOCS_VERSION=1.2.0"; exit 1; }
	@$(DOCS_NPM) --prefix "$(DOCS_DIR)" run refresh:docs-version -- "$(DOCS_VERSION)"

# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the Manager Docker image locally.
# CertManager is installed by default; skip with:
# - CERT_MANAGER_INSTALL_SKIP=true
KIND_CLUSTER ?= openbao-operator-test-e2e
KIND_NODE_IMAGE ?=
E2E_PARALLEL_NODES ?= 1
E2E_TIMEOUT ?= 1h
E2E_JUNIT_REPORT ?=
E2E_JSON_REPORT ?=
E2E_GOJSON_REPORT ?=
E2E_POLL_PROGRESS_AFTER ?=
E2E_FAIL_ON_EMPTY ?= false
E2E_KEEP_GOING ?= false
E2E_TRACE ?= false
E2E_NO_COLOR ?= false
E2E_FOCUS ?=
E2E_LABEL_FILTER ?=
E2E_SKIP_CLEANUP ?= false

PERF_NODE_IMAGE ?= kindest/node:v1.34.3
PERF_SAMPLES ?= 3
PERF_WARMUPS ?= 1
PERF_SCENARIO_TIMEOUT ?= 90m
PERF_SMOKE_SCENARIO_TIMEOUT ?= 45m
PERF_SMOKE_SCENARIOS ?= lifecycle-convergence
PERF_SCENARIOS ?= all
PERF_SCENARIOS_FILE ?= hack/perf/v2/scenarios.yaml
PERF_BASELINE_DIR ?= hack/perf/v2/baselines
PERF_POLICY_FILE ?= hack/perf/v2/policies/weekly.yaml
PERF_ARTIFACT_DIR ?= dist/perf
PERF_ENVIRONMENT ?= kind-v1.34.3

MUTATION_TARGET_PATH ?= ./internal/service/opslifecycle
MUTATION_PATHS ?= $(shell find ./internal -mindepth 1 -maxdepth 1 -type d | LC_ALL=C sort | paste -sd, -)
MUTATION_WORKERS ?= 1
MUTATION_TIMEOUT ?= 30
MUTATION_INCREMENTAL ?= false
MUTATION_TOP_SURVIVORS ?= 20
MUTATION_GOFLAGS ?= -p=1
MUTATION_GOMEMLIMIT ?= 8GiB

.PHONY: setup-test-e2e
setup-test-e2e: ## Set up a Kind cluster for e2e tests if it does not exist
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	@cluster_name="$(KIND_CLUSTER)"; \
	if $(KIND) get clusters | grep -qx "$$cluster_name"; then \
		echo "Kind cluster '$$cluster_name' already exists. Skipping creation."; \
	else \
		echo "Creating Kind cluster '$$cluster_name'..."; \
		if [ -n "$(KIND_NODE_IMAGE)" ]; then \
			$(KIND) create cluster --name "$$cluster_name" --image "$(KIND_NODE_IMAGE)" ; \
		else \
			$(KIND) create cluster --name "$$cluster_name" ; \
		fi; \
	fi

.PHONY: test-e2e
test-e2e: setup-test-e2e manifests generate fmt vet ginkgo ## Run the e2e tests. Expected an isolated environment using Kind. Use E2E_PARALLEL_NODES=N to run tests in parallel (default: 1). Use E2E_FOCUS="Backup" to run only specific tests. See Makefile for additional E2E_* variables.
	@for report in "$(E2E_JUNIT_REPORT)" "$(E2E_JSON_REPORT)" "$(E2E_GOJSON_REPORT)"; do \
		if [ -n "$$report" ]; then \
			mkdir -p "$$(dirname "$$report")"; \
		fi; \
	done; \
	GINKGO_FLAGS="-tags=e2e -v --timeout=$(E2E_TIMEOUT)"; \
	if [ -n "$(E2E_FOCUS)" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --focus=\"$(E2E_FOCUS)\""; \
	fi; \
	if [ -n "$(E2E_LABEL_FILTER)" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --label-filter=\"$(E2E_LABEL_FILTER)\""; \
	fi; \
	if [ "$(E2E_TRACE)" = "true" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --trace"; \
	fi; \
	if [ "$(E2E_NO_COLOR)" = "true" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --no-color"; \
	fi; \
	if [ -n "$(E2E_JUNIT_REPORT)" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --junit-report=$(E2E_JUNIT_REPORT)"; \
	fi; \
	if [ -n "$(E2E_JSON_REPORT)" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --json-report=$(E2E_JSON_REPORT)"; \
	fi; \
	if [ -n "$(E2E_GOJSON_REPORT)" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --gojson-report=$(E2E_GOJSON_REPORT)"; \
	fi; \
	if [ -n "$(E2E_POLL_PROGRESS_AFTER)" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --poll-progress-after=$(E2E_POLL_PROGRESS_AFTER)"; \
	fi; \
	if [ "$(E2E_FAIL_ON_EMPTY)" = "true" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --fail-on-empty"; \
	fi; \
	if [ "$(E2E_KEEP_GOING)" = "true" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --keep-going"; \
	fi; \
	eval KIND=$(KIND) KIND_CLUSTER=$(KIND_CLUSTER) "$(GINKGO)" $$GINKGO_FLAGS --procs=$(E2E_PARALLEL_NODES) ./test/e2e/
	@if [ "$(E2E_SKIP_CLEANUP)" != "true" ]; then \
		$(MAKE) cleanup-test-e2e; \
	else \
		echo "E2E_SKIP_CLEANUP=true: Keeping Kind cluster $(KIND_CLUSTER) for debugging"; \
	fi

.PHONY: test-e2e-existing
test-e2e-existing: manifests generate fmt vet ginkgo ## Run the e2e tests against an existing cluster (e.g. OpenShift Local/CRC). Requires KUBECONFIG set and E2E_OPERATOR_IMAGE pointing to a pullable image. Use E2E_LABEL_FILTER to run a subset (e.g. 'openshift').
	@for report in "$(E2E_JUNIT_REPORT)" "$(E2E_JSON_REPORT)" "$(E2E_GOJSON_REPORT)"; do \
		if [ -n "$$report" ]; then \
			mkdir -p "$$(dirname "$$report")"; \
		fi; \
	done; \
	GO_TEST_FLAGS="-tags=e2e -v -ginkgo.v -ginkgo.timeout=$(E2E_TIMEOUT)"; \
	if [ -n "$(E2E_FOCUS)" ]; then \
		GO_TEST_FLAGS="$$GO_TEST_FLAGS -ginkgo.focus=\"$(E2E_FOCUS)\""; \
	fi; \
	if [ -n "$(E2E_LABEL_FILTER)" ]; then \
		GO_TEST_FLAGS="$$GO_TEST_FLAGS -ginkgo.label-filter=\"$(E2E_LABEL_FILTER)\""; \
	fi; \
	if [ "$(E2E_TRACE)" = "true" ]; then \
		GO_TEST_FLAGS="$$GO_TEST_FLAGS -ginkgo.trace"; \
	fi; \
	if [ -n "$(E2E_JUNIT_REPORT)" ]; then \
		GO_TEST_FLAGS="$$GO_TEST_FLAGS -ginkgo.junit-report=$(E2E_JUNIT_REPORT)"; \
	fi; \
	if [ -n "$(E2E_JSON_REPORT)" ]; then \
		GO_TEST_FLAGS="$$GO_TEST_FLAGS -ginkgo.json-report=$(E2E_JSON_REPORT)"; \
	fi; \
	if [ -n "$(E2E_GOJSON_REPORT)" ]; then \
		GO_TEST_FLAGS="$$GO_TEST_FLAGS -ginkgo.gojson-report=$(E2E_GOJSON_REPORT)"; \
	fi; \
	if [ -n "$(E2E_POLL_PROGRESS_AFTER)" ]; then \
		GO_TEST_FLAGS="$$GO_TEST_FLAGS -ginkgo.poll-progress-after=$(E2E_POLL_PROGRESS_AFTER)"; \
	fi; \
	if [ "$(E2E_FAIL_ON_EMPTY)" = "true" ]; then \
		GO_TEST_FLAGS="$$GO_TEST_FLAGS -ginkgo.fail-on-empty"; \
	fi; \
	eval E2E_USE_EXISTING_CLUSTER=true go test ./test/e2e/ $$GO_TEST_FLAGS

.PHONY: test-e2e-ci
test-e2e-ci: ginkgo ## Run the e2e tests in CI mode (does not modify files).
	@for report in "$(E2E_JUNIT_REPORT)" "$(E2E_JSON_REPORT)" "$(E2E_GOJSON_REPORT)"; do \
		if [ -n "$$report" ]; then \
			mkdir -p "$$(dirname "$$report")"; \
		fi; \
	done; \
	GINKGO_FLAGS="-tags=e2e -v --timeout=$(E2E_TIMEOUT)"; \
	if [ -n "$(E2E_FOCUS)" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --focus=\"$(E2E_FOCUS)\""; \
	fi; \
	if [ -n "$(E2E_LABEL_FILTER)" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --label-filter=\"$(E2E_LABEL_FILTER)\""; \
	fi; \
	if [ "$(E2E_TRACE)" = "true" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --trace"; \
	fi; \
	if [ "$(E2E_NO_COLOR)" = "true" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --no-color"; \
	fi; \
	if [ -n "$(E2E_JUNIT_REPORT)" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --junit-report=$(E2E_JUNIT_REPORT)"; \
	fi; \
	if [ -n "$(E2E_JSON_REPORT)" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --json-report=$(E2E_JSON_REPORT)"; \
	fi; \
	if [ -n "$(E2E_GOJSON_REPORT)" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --gojson-report=$(E2E_GOJSON_REPORT)"; \
	fi; \
	if [ -n "$(E2E_POLL_PROGRESS_AFTER)" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --poll-progress-after=$(E2E_POLL_PROGRESS_AFTER)"; \
	fi; \
	if [ "$(E2E_FAIL_ON_EMPTY)" = "true" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --fail-on-empty"; \
	fi; \
	if [ "$(E2E_KEEP_GOING)" = "true" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --keep-going"; \
	fi; \
	eval KIND=$(KIND) KIND_CLUSTER=$(KIND_CLUSTER) "$(GINKGO)" $$GINKGO_FLAGS --procs=$(E2E_PARALLEL_NODES) ./test/e2e/
	@if [ "$(E2E_SKIP_CLEANUP)" != "true" ]; then \
		$(MAKE) cleanup-test-e2e; \
	else \
		echo "E2E_SKIP_CLEANUP=true: Keeping Kind cluster $(KIND_CLUSTER) for debugging"; \
	fi

.PHONY: e2e-catalog
e2e-catalog: ginkgo ## Generate a catalog of E2E suites, specs, labels, and By-steps under test/e2e/catalog/.
	@GOFLAGS="$(GOFLAGS_VENDOR)" go run ./hack/tools/e2e_catalog \
		--ginkgo "$(GINKGO)" \
		--input-dir test/e2e \
		--output-dir test/e2e/catalog

.PHONY: e2e-manifest-validate
e2e-manifest-validate: ## Validate test/e2e/suites.yaml against the generated E2E catalog.
	@GOFLAGS="$(GOFLAGS_VENDOR)" go run ./hack/tools/e2e_manifest \
		--manifest test/e2e/suites.yaml \
		--catalog test/e2e/catalog/cases.json

.PHONY: e2e-ci-matrix
e2e-ci-matrix: ## Generate the GitHub Actions E2E matrix from test/e2e/suites.yaml.
	@GOFLAGS="$(GOFLAGS_VENDOR)" go run ./hack/tools/e2e_plan \
		--manifest test/e2e/suites.yaml \
		--format github-matrix

.PHONY: e2e-nightly-matrix
e2e-nightly-matrix: ## Generate the GitHub Actions nightly E2E matrix. Set E2E_NIGHTLY_PROFILE, E2E_NIGHTLY_LANE, or E2E_NIGHTLY_KUBERNETES.
	@args=""; \
	if [ -n "$(E2E_NIGHTLY_LANE)" ] && [ "$(E2E_NIGHTLY_LANE)" != "all" ]; then args="$${args} --lane $(E2E_NIGHTLY_LANE)"; fi; \
	if [ -n "$(E2E_NIGHTLY_KUBERNETES)" ] && [ "$(E2E_NIGHTLY_KUBERNETES)" != "all" ]; then args="$${args} --kubernetes $(E2E_NIGHTLY_KUBERNETES)"; fi; \
	GOFLAGS="$(GOFLAGS_VENDOR)" go run ./hack/tools/e2e_plan \
		--manifest test/e2e/suites.yaml \
		--format github-nightly-matrix \
		--profile "$(or $(E2E_NIGHTLY_PROFILE),daily)" \
		$${args}

.PHONY: e2e-release-matrix
e2e-release-matrix: ## Generate the GitHub Actions release-gate E2E matrix. Set E2E_RELEASE_LANE or E2E_RELEASE_KUBERNETES.
	@args=""; \
	if [ -n "$(E2E_RELEASE_LANE)" ] && [ "$(E2E_RELEASE_LANE)" != "all" ]; then args="$${args} --lane $(E2E_RELEASE_LANE)"; fi; \
	if [ -n "$(E2E_RELEASE_KUBERNETES)" ] && [ "$(E2E_RELEASE_KUBERNETES)" != "all" ]; then args="$${args} --kubernetes $(E2E_RELEASE_KUBERNETES)"; fi; \
	GOFLAGS="$(GOFLAGS_VENDOR)" go run ./hack/tools/e2e_plan \
		--manifest test/e2e/suites.yaml \
		--format github-nightly-matrix \
		--profile release-gate \
		$${args}

.PHONY: e2e-ci-matrix-validate
e2e-ci-matrix-validate: ## Validate that the GitHub Actions E2E matrix can be generated.
	@GOFLAGS="$(GOFLAGS_VENDOR)" go run ./hack/tools/e2e_plan \
		--manifest test/e2e/suites.yaml \
		--format github-matrix >/dev/null

.PHONY: e2e-nightly-matrix-validate
e2e-nightly-matrix-validate: ## Validate that the nightly E2E matrices can be generated.
	@GOFLAGS="$(GOFLAGS_VENDOR)" go run ./hack/tools/e2e_plan \
		--manifest test/e2e/suites.yaml \
		--format github-nightly-matrix \
		--profile daily >/dev/null
	@GOFLAGS="$(GOFLAGS_VENDOR)" go run ./hack/tools/e2e_plan \
		--manifest test/e2e/suites.yaml \
		--format github-nightly-matrix \
		--profile weekly-full >/dev/null

.PHONY: e2e-release-matrix-validate
e2e-release-matrix-validate: ## Validate that the release-gate E2E matrix can be generated.
	@GOFLAGS="$(GOFLAGS_VENDOR)" go run ./hack/tools/e2e_plan \
		--manifest test/e2e/suites.yaml \
		--format github-nightly-matrix \
		--profile release-gate >/dev/null
	@GOFLAGS="$(GOFLAGS_VENDOR)" go run ./hack/tools/e2e_plan \
		--manifest test/e2e/suites.yaml \
		--format github-nightly-matrix \
		--profile release-gate \
		--lane core \
		--kubernetes 1.35.1 >/dev/null

.PHONY: verify-e2e-manifest
verify-e2e-manifest: e2e-catalog e2e-manifest-validate e2e-ci-matrix-validate e2e-nightly-matrix-validate e2e-release-matrix-validate ## Verify the E2E catalog and suite manifest are up-to-date.
	@{ \
		git diff --exit-code -- test/e2e/catalog test/e2e/suites.yaml; \
	} || { \
		echo "E2E catalog or suite manifest is out of date. Run 'make e2e-catalog e2e-manifest-validate' and commit the result."; \
		git --no-pager diff -- test/e2e/catalog test/e2e/suites.yaml; \
		exit 1; \
	}

.PHONY: perf-baseline
perf-baseline: perf-v2-capture ## Capture performance baseline samples and write v2 distribution baselines.

.PHONY: perf-v2-capture
perf-v2-capture: ## Capture v2 performance samples and update per-scenario distribution baselines.
	go run ./hack/perfcheck capture \
		--samples="$(PERF_SAMPLES)" \
		--warmups="$(PERF_WARMUPS)" \
		--scenarios="$(PERF_SCENARIOS)" \
		--scenario-manifest="$(PERF_SCENARIOS_FILE)" \
		--kind="$(KIND)" \
		--node-image="$(PERF_NODE_IMAGE)" \
		--baseline-dir="$(PERF_BASELINE_DIR)" \
		--artifact-dir="$(PERF_ARTIFACT_DIR)" \
		--environment="$(PERF_ENVIRONMENT)" \
		--scenario-timeout="$(PERF_SCENARIO_TIMEOUT)"

.PHONY: verify-perf
verify-perf: perf-v2-verify ## Run v2 performance verification against committed distribution baselines.

.PHONY: perf-v2-verify
perf-v2-verify: ## Run v2 performance verification against committed distribution baselines.
	go run ./hack/perfcheck verify \
		--samples="$(PERF_SAMPLES)" \
		--warmups="$(PERF_WARMUPS)" \
		--scenarios="$(PERF_SCENARIOS)" \
		--scenario-manifest="$(PERF_SCENARIOS_FILE)" \
		--kind="$(KIND)" \
		--node-image="$(PERF_NODE_IMAGE)" \
		--baseline-dir="$(PERF_BASELINE_DIR)" \
		--policy="$(PERF_POLICY_FILE)" \
		--artifact-dir="$(PERF_ARTIFACT_DIR)" \
		--environment="$(PERF_ENVIRONMENT)" \
		--scenario-timeout="$(PERF_SCENARIO_TIMEOUT)"

.PHONY: verify-perf-smoke
verify-perf-smoke: perf-v2-smoke ## Run a lightweight v2 performance smoke gate (PR-focused).

.PHONY: perf-v2-smoke
perf-v2-smoke: ## Run a lightweight v2 performance smoke gate (PR-focused).
	go run ./hack/perfcheck verify \
		--samples=1 \
		--warmups=0 \
		--scenarios="$(PERF_SMOKE_SCENARIOS)" \
		--scenario-manifest="$(PERF_SCENARIOS_FILE)" \
		--kind="$(KIND)" \
		--node-image="$(PERF_NODE_IMAGE)" \
		--baseline-dir="$(PERF_BASELINE_DIR)" \
		--policy="$(PERF_POLICY_FILE)" \
		--artifact-dir="$(PERF_ARTIFACT_DIR)" \
		--environment="$(PERF_ENVIRONMENT)" \
		--scenario-timeout="$(PERF_SMOKE_SCENARIO_TIMEOUT)"

.PHONY: perf-v2-report
perf-v2-report: ## Render a v2 performance report from existing sample artifacts.
	go run ./hack/perfcheck report \
		--scenario-manifest="$(PERF_SCENARIOS_FILE)" \
		--scenarios="$(PERF_SCENARIOS)" \
		--baseline-dir="$(PERF_BASELINE_DIR)" \
		--policy="$(PERF_POLICY_FILE)" \
		--artifact-dir="$(PERF_ARTIFACT_DIR)" \
		--environment="$(PERF_ENVIRONMENT)"

.PHONY: mutation-smoke
mutation-smoke: gomu ## Run a fast mutation smoke check (operation lifecycle package).
	@out="dist/mutation/smoke-$$(date -u +%Y%m%dT%H%M%SZ)"; \
	GOMU_BIN="$(GOMU)" bash hack/ci/run-gomu.sh \
		--path "./internal/service/opslifecycle" \
		--workers "1" \
		--timeout "20" \
		--incremental "false" \
		--ci-mode "false" \
		--fail-on-gate "false" \
		--output-dir "$$out"; \
	echo "Mutation smoke artifacts: $$out"

.PHONY: mutation-target
mutation-target: gomu ## Run mutation testing for one target path (set MUTATION_TARGET_PATH=./internal/<pkg>).
	@out="dist/mutation/target-$$(date -u +%Y%m%dT%H%M%SZ)"; \
	GOMU_BIN="$(GOMU)" bash hack/ci/run-gomu.sh \
		--path "$(MUTATION_TARGET_PATH)" \
		--workers "$(MUTATION_WORKERS)" \
		--timeout "$(MUTATION_TIMEOUT)" \
		--incremental "$(MUTATION_INCREMENTAL)" \
		--go-flags "$(MUTATION_GOFLAGS)" \
		--go-mem-limit "$(MUTATION_GOMEMLIMIT)" \
		--ci-mode "false" \
		--fail-on-gate "false" \
		--top-survivors "$(MUTATION_TOP_SURVIVORS)" \
		--output-dir "$$out"; \
	echo "Mutation target artifacts: $$out"

.PHONY: mutation-local
mutation-local: gomu ## Run broad local mutation testing (report-only mode).
	@out="dist/mutation/local-$$(date -u +%Y%m%dT%H%M%SZ)"; \
	GOMU_BIN="$(GOMU)" bash hack/ci/run-gomu.sh \
		--path "$(MUTATION_PATHS)" \
		--workers "$(MUTATION_WORKERS)" \
		--timeout "$(MUTATION_TIMEOUT)" \
		--incremental "$(MUTATION_INCREMENTAL)" \
		--go-flags "$(MUTATION_GOFLAGS)" \
		--go-mem-limit "$(MUTATION_GOMEMLIMIT)" \
		--ci-mode "false" \
		--fail-on-gate "false" \
		--top-survivors "$(MUTATION_TOP_SURVIVORS)" \
		--output-dir "$$out"; \
	echo "Mutation local artifacts: $$out"

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Tear down the Kind cluster used for e2e tests
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}; \
	echo "Deleting Kind cluster '$(KIND_CLUSTER)'"; \
	$(KIND) delete cluster --name "$(KIND_CLUSTER)" || true

BENCH_ARTIFACT_DIR ?= dist/bench
BENCH_PKG ?= ./...
BENCH_FILTER ?= .
BENCH_COUNT ?= 10
GO_LICENSES_ALLOWED ?= Apache-2.0 BSD-2-Clause BSD-3-Clause ISC MIT MPL-2.0 Unicode-DFS-2016
GO_LICENSES_IGNORE ?= github.com/dc-tec/openbao-operator
GO_LICENSES_PACKAGE_TARGETS ?= ./cmd/controller ./cmd/bao-backup ./cmd/bao-upgrade ./cmd/provisioner
LICENSE_REPORT_DIR ?= dist/licenses
go_licenses_empty :=
go_licenses_space := $(go_licenses_empty) $(go_licenses_empty)
go_licenses_comma := ,
GO_LICENSES_ALLOWED_CSV := $(subst $(go_licenses_space),$(go_licenses_comma),$(strip $(GO_LICENSES_ALLOWED)))
SEMGREP_ARTIFACT_DIR ?= dist/semgrep
SEMGREP_CONFIG_FLAGS ?= --config p/default --config .semgrep/rules
SEMGREP_TARGETS ?= ./cmd ./internal ./api ./hack ./config ./.github
SEMGREP_OUTPUT_JSON ?= $(SEMGREP_ARTIFACT_DIR)/semgrep.json

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	"$(GOLANGCI_LINT)" run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	"$(GOLANGCI_LINT)" run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	"$(GOLANGCI_LINT)" config verify

.PHONY: lint-ci
lint-ci: lint-config lint verify-arch-policy lint-testonly-exports test-ast lint-ast ## Run CI lint gates (golangci-lint + test-only export audit + ast-grep tests/scans).

.PHONY: vulncheck
vulncheck: govulncheck ## Run govulncheck to scan for known vulnerabilities (production code only). Findings listed in .govulnignore are ignored. Set VULNCHECK_SHOW_IGNORED=true to print traces even if all findings are ignored.
	@go run ./hack/govulncheck_wrapper/ -govulncheck "$(GOVULNCHECK)" -ignore .govulnignore -show-ignored="$${VULNCHECK_SHOW_IGNORED:-false}" ./...

.PHONY: semgrep-rules-test
semgrep-rules-test: semgrep ## Validate repo-local Semgrep custom rules against test fixtures.
	"$(SEMGREP)" scan --test --config .semgrep/rules .semgrep/tests

.PHONY: semgrep-scan
semgrep-scan: semgrep ## Run Semgrep against security-relevant code, config, and CI surfaces (report-only).
	"$(SEMGREP)" scan --metrics=off $(SEMGREP_CONFIG_FLAGS) $(SEMGREP_TARGETS)

.PHONY: semgrep-ci
semgrep-ci: semgrep semgrep-rules-test ## Run the blocking CI-equivalent Semgrep scan and write JSON output.
	@mkdir -p "$(SEMGREP_ARTIFACT_DIR)"
	"$(SEMGREP)" scan --metrics=off --error --json --output "$(SEMGREP_OUTPUT_JSON)" $(SEMGREP_CONFIG_FLAGS) $(SEMGREP_TARGETS)

.PHONY: license-check
license-check: verify-vendor go-licenses ## Verify shipped Go dependencies use approved licenses.
	@GOFLAGS="$(GOFLAGS_VENDOR)" "$(GO_LICENSES)" check \
		--allowed_licenses="$(GO_LICENSES_ALLOWED_CSV)" \
		--ignore "$(GO_LICENSES_IGNORE)" \
		$(GO_LICENSES_PACKAGE_TARGETS)

.PHONY: license-report
license-report: verify-vendor go-licenses ## Write a CSV inventory for shipped Go dependency licenses to dist/licenses/.
	@mkdir -p "$(LICENSE_REPORT_DIR)"
	@GOFLAGS="$(GOFLAGS_VENDOR)" "$(GO_LICENSES)" report \
		--ignore "$(GO_LICENSES_IGNORE)" \
		$(GO_LICENSES_PACKAGE_TARGETS) \
		> "$(LICENSE_REPORT_DIR)/go-licenses-report.csv" \
		2> "$(LICENSE_REPORT_DIR)/go-licenses-report.stderr.log"
	@echo "License report written to $(LICENSE_REPORT_DIR)/go-licenses-report.csv"

.PHONY: security-ci
security-ci: vulncheck license-check semgrep-ci security-scan-fs ## Run CI-equivalent security checks.

.PHONY: security-scan
security-scan: security-scan-fs security-scan-image ## Run Trivy security scans (filesystem and container image).

.PHONY: security-scan-fs
security-scan-fs: ## Run the Trivy filesystem scan used by CI.
	# Keep local scans aligned with CI (see .github/workflows/ci.yml):
	# - Use "misconfig" (not deprecated "config")
	# - Explicitly load ignore rules from .trivyignore
	# - Render Helm charts against a modern Kubernetes version
	trivy fs \
		--scanners vuln,misconfig \
		--severity HIGH,CRITICAL \
		--ignore-unfixed \
		--exit-code 1 \
		--ignorefile .trivyignore \
		--skip-version-check \
		--helm-kube-version 1.34.0 \
		--skip-files config/rbac/provisioner_minimal_role.yaml \
		--skip-files charts/openbao-operator/templates/rbac/provisioner-clusterroles.yaml \
		--skip-files config/rbac/single_tenant_clusterrole.yaml \
		--skip-files config/overlays/single-tenant/single_tenant_clusterrole.yaml \
		--skip-files config/overlays/single-tenant-custom-identity/single_tenant_clusterrole.yaml \
		--skip-files dist/install.yaml \
		--skip-dirs test/manifests \
		--skip-dirs vendor \
		--skip-dirs website/node_modules \
		--skip-dirs .github/tools/node_modules \
		--skip-dirs bin \
		.

.PHONY: security-scan-image
security-scan-image: ## Run a Trivy image scan against IMG.
	trivy image \
		--severity HIGH,CRITICAL \
		--ignore-unfixed \
		--exit-code 1 \
		--skip-version-check \
		${IMG}

.PHONY: debug-controller
debug-controller: dlv manifests generate fmt vet ## Debug the cluster controller locally with Delve.
	"$(DLV)" debug ./cmd/main.go -- controller

.PHONY: debug-provisioner
debug-provisioner: dlv manifests generate fmt vet ## Debug the provisioner locally with Delve.
	"$(DLV)" debug ./cmd/main.go -- provisioner

.PHONY: debug-test
debug-test: dlv ## Debug a Go test package with Delve. Set PKG=./path and optionally TEST=Regex.
	@if [ -z "$(PKG)" ]; then \
		echo "Error: PKG is required. Example: make debug-test PKG=./internal/service/upgrade TEST=TestManager"; \
		exit 1; \
	fi
	@args='test $(PKG)'; \
	if [ -n "$(TEST)" ]; then \
		exec "$(DLV)" $$args -- -test.run='$(TEST)'; \
	else \
		exec "$(DLV)" $$args; \
	fi

.PHONY: air-controller
air-controller: air ## Run the cluster controller with live reload via Air.
	"$(AIR)" -c hack/dev/air.controller.toml

.PHONY: air-provisioner
air-provisioner: air ## Run the provisioner with live reload via Air.
	"$(AIR)" -c hack/dev/air.provisioner.toml

.PHONY: bench
bench: ## Run Go benchmarks. Override BENCH_PKG, BENCH_FILTER, and BENCH_COUNT as needed.
	GOFLAGS="$(GOFLAGS_VENDOR)" go test -run='^$$' -bench="$(BENCH_FILTER)" -benchmem -count="$(BENCH_COUNT)" $(BENCH_PKG)

.PHONY: bench-save
bench-save: ## Run Go benchmarks and save the output under dist/bench/.
	@mkdir -p "$(BENCH_ARTIFACT_DIR)"
	@out="$${BENCH_OUT:-$(BENCH_ARTIFACT_DIR)/bench-$$(date -u +%Y%m%dT%H%M%SZ).txt}"; \
	echo "Writing benchmark output to $$out"; \
	GOFLAGS="$(GOFLAGS_VENDOR)" go test -run='^$$' -bench="$(BENCH_FILTER)" -benchmem -count="$(BENCH_COUNT)" $(BENCH_PKG) | tee "$$out"

.PHONY: bench-compare
bench-compare: benchstat ## Compare benchmark result files with benchstat. Set OLD=/path/to/old.txt NEW=/path/to/new.txt.
	@if [ -z "$(OLD)" ] || [ -z "$(NEW)" ]; then \
		echo "Error: OLD and NEW are required. Example: make bench-compare OLD=dist/bench/old.txt NEW=dist/bench/new.txt"; \
		exit 1; \
	fi
	"$(BENCHSTAT)" "$(OLD)" "$(NEW)"
