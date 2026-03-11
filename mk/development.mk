##@ Development

.PHONY: bootstrap
bootstrap: controller-gen kustomize crd-ref-docs envtest setup-envtest golangci-lint ginkgo govulncheck gomu gotestsum dlv air benchstat ## Install repo-managed tools and local development dependencies.
	@if command -v "$(NPM)" >/dev/null 2>&1; then \
		$(MAKE) ast-grep; \
	else \
		echo "Skipping ast-grep bootstrap because npm is not available."; \
	fi
	@if command -v "$(DOCS_PYTHON)" >/dev/null 2>&1 && "$(DOCS_PYTHON)" -m venv --help >/dev/null 2>&1; then \
		$(MAKE) docs-deps; \
	else \
		echo "Skipping docs bootstrap because python3 with venv support is not available."; \
	fi
	@echo "Bootstrap complete."
	@echo "Run 'make doctor' to validate external prerequisites."

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
helm-test: helm-sync helm-lint ## Test the Helm chart (lint, template, and dry-run install).
	@echo "Testing Helm chart: templating with default values..."
	@helm template openbao-operator charts/openbao-operator \
		--namespace openbao-operator-system \
		--include-crds > /dev/null
	@echo "Testing Helm chart: templating with multi-tenant mode..."
	@helm template openbao-operator charts/openbao-operator \
		--namespace openbao-operator-system \
		--include-crds \
		--set tenancy.mode=multi > /dev/null
	@echo "Testing Helm chart: templating with single-tenant mode..."
	@helm template openbao-operator charts/openbao-operator \
		--namespace openbao-operator-system \
		--include-crds \
		--set tenancy.mode=single \
		--set tenancy.targetNamespace=openbao-system > /dev/null
	@echo "Testing Helm chart: dry-run install with default values..."
	@helm install openbao-operator charts/openbao-operator \
		--namespace openbao-operator-system \
		--create-namespace \
		--dry-run > /dev/null
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

DOCS_VENV ?= .venv-docs
DOCS_PYTHON ?= python3
DOCS_PIP ?= $(DOCS_VENV)/bin/pip
DOCS_MKDOCS ?= $(DOCS_VENV)/bin/mkdocs
TEST_ARTIFACT_DIR ?= dist/test
GOTESTSUM_FORMAT ?= pkgname

.PHONY: docs-deps
docs-deps: ## Install MkDocs tooling in a local venv (CI-equivalent).
	@$(DOCS_PYTHON) -m venv "$(DOCS_VENV)"
	@$(DOCS_PIP) install --upgrade pip
	@$(DOCS_PIP) install mkdocs-material mike

.PHONY: docs-build
docs-build: docs-deps ## Build docs locally (CI-equivalent; strict). Writes ./site/.
	@$(DOCS_MKDOCS) build --strict

.PHONY: docs-serve
docs-serve: docs-deps ## Serve docs locally. http://localhost:8000
	@$(DOCS_MKDOCS) serve -a 0.0.0.0:8000

# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the Manager Docker image locally.
# CertManager is installed by default; skip with:
# - CERT_MANAGER_INSTALL_SKIP=true
KIND_CLUSTER ?= openbao-operator-test-e2e
KIND_NODE_IMAGE ?=
E2E_PARALLEL_NODES ?= 1
E2E_TIMEOUT ?= 1h
E2E_JUNIT_REPORT ?=
E2E_KEEP_GOING ?= false
E2E_TRACE ?= false
E2E_NO_COLOR ?= false
E2E_FOCUS ?=
E2E_LABEL_FILTER ?=
E2E_SKIP_CLEANUP ?= false

PERF_NODE_IMAGE ?= kindest/node:v1.34.3
PERF_RUNS ?= 5
PERF_SCENARIO_TIMEOUT ?= 90m
PERF_SMOKE_SCENARIO_TIMEOUT ?= 45m
PERF_SMOKE_SCENARIOS ?= lifecycle
PERF_BASELINE_OUT ?= hack/perf/baseline/kind-v1.34.3-baseline.json
PERF_THRESHOLDS_OUT ?= hack/perf/thresholds/kind-v1.34.3.yaml

MUTATION_TARGET_PATH ?= ./internal/adapter/operationlock
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
	@GINKGO_FLAGS="-tags=e2e -v --timeout=$(E2E_TIMEOUT)"; \
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
	@GO_TEST_FLAGS="-tags=e2e -v -ginkgo.v -ginkgo.timeout=$(E2E_TIMEOUT)"; \
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
	eval E2E_USE_EXISTING_CLUSTER=true go test ./test/e2e/ $$GO_TEST_FLAGS

.PHONY: test-e2e-ci
test-e2e-ci: ginkgo ## Run the e2e tests in CI mode (does not modify files).
	@GINKGO_FLAGS="-tags=e2e -v --timeout=$(E2E_TIMEOUT)"; \
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

.PHONY: perf-baseline
perf-baseline: ## Capture performance baseline (5 runs/scenario by default) and regenerate thresholds.
	go run ./hack/perfcheck capture \
		--runs="$(PERF_RUNS)" \
		--scenarios=all \
		--kind="$(KIND)" \
		--node-image="$(PERF_NODE_IMAGE)" \
		--baseline-out="$(PERF_BASELINE_OUT)" \
		--thresholds-out="$(PERF_THRESHOLDS_OUT)" \
		--scenario-timeout="$(PERF_SCENARIO_TIMEOUT)"

.PHONY: verify-perf
verify-perf: ## Run performance regression gate against committed thresholds.
	go run ./hack/perfcheck verify \
		--scenarios=all \
		--kind="$(KIND)" \
		--node-image="$(PERF_NODE_IMAGE)" \
		--thresholds="$(PERF_THRESHOLDS_OUT)" \
		--scenario-timeout="$(PERF_SCENARIO_TIMEOUT)"

.PHONY: verify-perf-smoke
verify-perf-smoke: ## Run a lightweight performance smoke gate (PR-focused).
	go run ./hack/perfcheck verify \
		--scenarios="$(PERF_SMOKE_SCENARIOS)" \
		--kind="$(KIND)" \
		--node-image="$(PERF_NODE_IMAGE)" \
		--thresholds="$(PERF_THRESHOLDS_OUT)" \
		--scenario-timeout="$(PERF_SMOKE_SCENARIO_TIMEOUT)"

.PHONY: mutation-smoke
mutation-smoke: gomu ## Run a fast mutation smoke check (operationlock package).
	@out="dist/mutation/smoke-$$(date -u +%Y%m%dT%H%M%SZ)"; \
	GOMU_BIN="$(GOMU)" bash hack/ci/run-gomu.sh \
		--path "./internal/adapter/operationlock" \
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
lint-ci: lint-config lint verify-arch-policy test-ast lint-ast ## Run CI lint gates (golangci-lint + ast-grep tests/scans).

.PHONY: vulncheck
vulncheck: govulncheck ## Run govulncheck to scan for known vulnerabilities (production code only). Findings listed in .govulnignore are ignored. Set VULNCHECK_SHOW_IGNORED=true to print traces even if all findings are ignored.
	@go run ./hack/govulncheck_wrapper/ -govulncheck "$(GOVULNCHECK)" -ignore .govulnignore -show-ignored="$${VULNCHECK_SHOW_IGNORED:-false}" ./...

.PHONY: security-ci
security-ci: vulncheck security-scan-fs ## Run CI-equivalent security checks.

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
		--skip-files dist/install.yaml \
		--skip-dirs test/manifests \
		--skip-dirs vendor \
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
