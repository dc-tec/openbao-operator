# Image URL to use all building/pushing image targets
IMG ?= controller:latest

# OPERATOR_VERSION is injected into the controller/provisioner Deployments.
# This is used by the controller to derive version-matched helper images.
OPERATOR_VERSION ?= v0.0.0

# INIT_IMG is the image for the config-init helper used as an init container
# in OpenBao pods to render the final config.hcl from the template.
# When running init-image targets, you can either set INIT_IMG explicitly:
#   make docker-build-init INIT_IMG=localhost:5000/openbao-config-init:dev
# or reuse IMG:
#   make docker-build-init IMG=localhost:5000/openbao-config-init:dev
INIT_IMG ?= openbao-config-init:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

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
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "The following files are not gofmt'd:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet ## Run unit tests (fast, no envtest).
	go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: test-ci
test-ci: manifests generate vet setup-envtest ## Run unit + integration tests in CI mode (does not modify tracked files).
	KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" go test $$(go list ./... | grep -v /e2e) -tags=integration -coverprofile cover.out

.PHONY: test-integration
test-integration: manifests generate vet setup-envtest ## Run envtest-based integration tests (envtest; requires -tags=integration).
	KUBEBUILDER_ASSETS="$(shell "$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path)" go test $$(go list ./... | grep -v /e2e) -tags=integration -count=1 -v

.PHONY: verify-tidy
verify-tidy: ## Verify go.mod/go.sum are tidy (does not modify tracked files).
	@go mod tidy
	@{ \
		git diff --exit-code -- go.mod go.sum; \
	} || { \
		echo "go.mod/go.sum are not tidy. Run 'go mod tidy' and commit the result."; \
		git --no-pager diff -- go.mod go.sum; \
		exit 1; \
	}

.PHONY: rbac-sync
rbac-sync: ## Sync the provisioner delegate RBAC template from Go sources.
	@go run ./hack/gen-rbac

.PHONY: verify-rbac-sync
verify-rbac-sync: ## Verify provisioner delegate RBAC template is in sync (does not modify tracked files).
	@go run ./hack/gen-rbac
	@{ \
		git diff --exit-code -- config/rbac/provisioner_delegate_clusterrole.yaml; \
	} || { \
		echo "RBAC delegate template is out of date. Run 'make rbac-sync' and commit the result."; \
		git --no-pager diff -- config/rbac/provisioner_delegate_clusterrole.yaml; \
		exit 1; \
	}

.PHONY: verify-generated
verify-generated: manifests generate verify-rbac-sync ## Verify generated artifacts are up-to-date (does not modify tracked files).
	@{ \
		git diff --exit-code -- api/v1alpha1 config/crd/bases; \
	} || { \
		echo "Generated artifacts are out of date. Run 'make manifests generate' and commit the result."; \
		git --no-pager diff -- api/v1alpha1 config/crd/bases; \
		exit 1; \
	}

.PHONY: verify-openbao-config-compat
verify-openbao-config-compat: ## Validate generated HCL fixtures against upstream OpenBao config parser (semantic).
	@bash hack/ci/openbao-config-compat.sh 2.4.0 2.4.4

.PHONY: report-openbao-config-schema-drift
report-openbao-config-schema-drift: ## Report upstream OpenBao config schema drift across the supported range (non-failing).
	@REPORT_SCHEMA_DRIFT=true bash hack/ci/openbao-config-compat.sh 2.4.0 2.4.4

.PHONY: report-openbao-operator-schema-drift
report-openbao-operator-schema-drift: ## Report operator-vs-upstream OpenBao config schema drift (non-failing).
	@go run ./hack/tools/openbao_operator_schema_drift --openbao-image-tag 2.4.4

.PHONY: changelog
changelog: ## Generate CHANGELOG.md from git history (local, non-CI).
	@go run ./hack/changelog --out CHANGELOG.md

.PHONY: changelog-all
changelog-all: ## Generate CHANGELOG.md including all git tags (local, non-CI).
	@go run ./hack/changelog --out CHANGELOG.md --all-tags

.PHONY: helm-sync
helm-sync: manifests ## Sync Helm chart from config/ (CRDs, admission policies).
	@go run ./hack/helmchart

.PHONY: verify-helm
verify-helm: helm-sync ## Verify Helm chart is up-to-date (does not modify tracked files).
	@{ \
		git diff --exit-code -- charts/openbao-operator/crds charts/openbao-operator/templates/admission; \
	} || { \
		echo "Helm chart is out of date. Run 'make helm-sync' and commit the result."; \
		git --no-pager diff -- charts/openbao-operator/crds charts/openbao-operator/templates/admission; \
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
test-update-golden: ## Update golden files for HCL generation tests. Run this when modifying internal/config/builder.go or related config generation logic.
	UPDATE_GOLDEN=true go test ./internal/config/... -v

.PHONY: verify-trusted-root
verify-trusted-root: ## Verify that trusted_root.json exists and is valid JSON.
	@if [ ! -f internal/security/trusted_root.json ]; then \
		echo "Error: trusted_root.json not found. Run 'make update-trusted-root' to download it."; \
		exit 1; \
	fi
	@python3 -m json.tool internal/security/trusted_root.json > /dev/null 2>&1 || { \
		echo "Error: trusted_root.json is not valid JSON. Run 'make update-trusted-root' to fix it."; \
		exit 1; \
	}
	@echo "trusted_root.json is valid"

DOCS_VENV ?= .venv-docs
DOCS_PYTHON ?= python3
DOCS_PIP ?= $(DOCS_VENV)/bin/pip
DOCS_MKDOCS ?= $(DOCS_VENV)/bin/mkdocs

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
# KIND_NODE_IMAGE pins the Kind node image (and thus Kubernetes version) used for E2E.
# Example: make test-e2e KIND_NODE_IMAGE=kindest/node:v1.34.3
KIND_NODE_IMAGE ?=
# E2E_PARALLEL_NODES controls the number of parallel nodes for e2e tests.
# Each parallel node uses its own Kind cluster ($(KIND_CLUSTER)-1, $(KIND_CLUSTER)-2, ...).
# Example: make test-e2e E2E_PARALLEL_NODES=4
E2E_PARALLEL_NODES ?= 1
# E2E_TIMEOUT sets the timeout for the entire test suite (default: 1h).
# Example: make test-e2e E2E_TIMEOUT=2h
E2E_TIMEOUT ?= 1h
# E2E_JUNIT_REPORT generates a JUnit XML report (useful for CI).
# Example: make test-e2e E2E_JUNIT_REPORT=test-results.xml
E2E_JUNIT_REPORT ?=
# E2E_KEEP_GOING continues running all tests even if some fail (useful for parallel runs).
# Set to true to enable. Example: make test-e2e E2E_KEEP_GOING=true
E2E_KEEP_GOING ?= false
# E2E_TRACE shows full stack traces on failures.
# Set to true to enable. Example: make test-e2e E2E_TRACE=true
E2E_TRACE ?= false
# E2E_NO_COLOR disables color output (useful for CI).
# Set to true to enable. Example: make test-e2e E2E_NO_COLOR=true
E2E_NO_COLOR ?= false
# E2E_FOCUS runs only tests matching the focus pattern (useful for running specific test suites).
# Example: make test-e2e E2E_FOCUS=Backup
E2E_FOCUS ?=
# E2E_LABEL_FILTER runs only tests whose Ginkgo v2 labels match the filter expression.
# Example: make test-e2e E2E_LABEL_FILTER='smoke && critical'
E2E_LABEL_FILTER ?=
# E2E_SKIP_CLEANUP skips the Kind cluster cleanup after tests (useful for debugging).
# Set to true to keep the cluster running. Example: make test-e2e E2E_SKIP_CLEANUP=true
E2E_SKIP_CLEANUP ?= false

.PHONY: setup-test-e2e
setup-test-e2e: ## Set up a Kind cluster for e2e tests if it does not exist
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	@i=1; \
	while [ $$i -le "$(E2E_PARALLEL_NODES)" ]; do \
		cluster_name="$(KIND_CLUSTER)-$$i"; \
		if $(KIND) get clusters | grep -qx "$$cluster_name"; then \
			echo "Kind cluster '$$cluster_name' already exists. Skipping creation."; \
		else \
			echo "Creating Kind cluster '$$cluster_name'..."; \
			if [ -n "$(KIND_NODE_IMAGE)" ]; then \
				$(KIND) create cluster --name "$$cluster_name" --image "$(KIND_NODE_IMAGE)" ; \
			else \
				$(KIND) create cluster --name "$$cluster_name" ; \
			fi; \
		fi; \
		i=$$((i+1)); \
	done

.PHONY: test-e2e
test-e2e: setup-test-e2e manifests generate fmt vet ginkgo ## Run the e2e tests. Expected an isolated environment using Kind. Use E2E_PARALLEL_NODES=N to run tests in parallel (default: 1). Use E2E_FOCUS="Backup" to run only specific tests. See Makefile for additional E2E_* variables.
	@GINKGO_FLAGS="-tags=e2e -v --timeout=$(E2E_TIMEOUT)"; \
	if [ -n "$(E2E_FOCUS)" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --focus=$(E2E_FOCUS)"; \
	fi; \
	if [ -n "$(E2E_LABEL_FILTER)" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --label-filter=$(E2E_LABEL_FILTER)"; \
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
	if [ "$(E2E_PARALLEL_NODES)" -eq 1 ]; then \
		GO_TEST_FLAGS="-tags=e2e -v -ginkgo.v -ginkgo.timeout=$(E2E_TIMEOUT)"; \
		if [ -n "$(E2E_FOCUS)" ]; then \
			GO_TEST_FLAGS="$$GO_TEST_FLAGS -ginkgo.focus=$(E2E_FOCUS)"; \
		fi; \
		if [ -n "$(E2E_LABEL_FILTER)" ]; then \
			GO_TEST_FLAGS="$$GO_TEST_FLAGS -ginkgo.label-filter=$(E2E_LABEL_FILTER)"; \
		fi; \
		if [ "$(E2E_TRACE)" = "true" ]; then \
			GO_TEST_FLAGS="$$GO_TEST_FLAGS -ginkgo.trace"; \
		fi; \
		if [ -n "$(E2E_JUNIT_REPORT)" ]; then \
			GO_TEST_FLAGS="$$GO_TEST_FLAGS -ginkgo.junit-report=$(E2E_JUNIT_REPORT)"; \
		fi; \
		KIND=$(KIND) KIND_CLUSTER=$(KIND_CLUSTER) go test $$GO_TEST_FLAGS ./test/e2e/; \
	else \
		KIND=$(KIND) KIND_CLUSTER=$(KIND_CLUSTER) "$(GINKGO)" $$GINKGO_FLAGS --procs=$(E2E_PARALLEL_NODES) ./test/e2e/; \
	fi
	@if [ "$(E2E_SKIP_CLEANUP)" != "true" ]; then \
		$(MAKE) cleanup-test-e2e; \
	else \
		echo "E2E_SKIP_CLEANUP=true: Keeping Kind cluster $(KIND_CLUSTER) for debugging"; \
	fi

.PHONY: test-e2e-ci
test-e2e-ci: setup-test-e2e manifests generate vet ginkgo ## Run the e2e tests in CI mode (does not modify files).
	@GINKGO_FLAGS="-tags=e2e -v --timeout=$(E2E_TIMEOUT)"; \
	if [ -n "$(E2E_FOCUS)" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --focus=$(E2E_FOCUS)"; \
	fi; \
	if [ -n "$(E2E_LABEL_FILTER)" ]; then \
		GINKGO_FLAGS="$$GINKGO_FLAGS --label-filter=$(E2E_LABEL_FILTER)"; \
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
	if [ "$(E2E_PARALLEL_NODES)" -eq 1 ]; then \
		GO_TEST_FLAGS="-tags=e2e -v -ginkgo.v -ginkgo.timeout=$(E2E_TIMEOUT)"; \
		if [ -n "$(E2E_FOCUS)" ]; then \
			GO_TEST_FLAGS="$$GO_TEST_FLAGS -ginkgo.focus=$(E2E_FOCUS)"; \
		fi; \
		if [ -n "$(E2E_LABEL_FILTER)" ]; then \
			GO_TEST_FLAGS="$$GO_TEST_FLAGS -ginkgo.label-filter=$(E2E_LABEL_FILTER)"; \
		fi; \
		if [ "$(E2E_TRACE)" = "true" ]; then \
			GO_TEST_FLAGS="$$GO_TEST_FLAGS -ginkgo.trace"; \
		fi; \
		if [ -n "$(E2E_JUNIT_REPORT)" ]; then \
			GO_TEST_FLAGS="$$GO_TEST_FLAGS -ginkgo.junit-report=$(E2E_JUNIT_REPORT)"; \
		fi; \
		KIND=$(KIND) KIND_CLUSTER=$(KIND_CLUSTER) go test $$GO_TEST_FLAGS ./test/e2e/; \
	else \
		KIND=$(KIND) KIND_CLUSTER=$(KIND_CLUSTER) "$(GINKGO)" $$GINKGO_FLAGS --procs=$(E2E_PARALLEL_NODES) ./test/e2e/; \
	fi
	@if [ "$(E2E_SKIP_CLEANUP)" != "true" ]; then \
		$(MAKE) cleanup-test-e2e; \
	else \
		echo "E2E_SKIP_CLEANUP=true: Keeping Kind cluster $(KIND_CLUSTER) for debugging"; \
	fi

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Tear down the Kind cluster used for e2e tests
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}; \
	base="$(KIND_CLUSTER)"; \
	clusters="$$( $(KIND) get clusters 2>/dev/null || true )"; \
	while IFS= read -r cluster; do \
		[ -z "$$cluster" ] && continue; \
		if [ "$$cluster" = "$$base" ]; then \
			echo "Deleting Kind cluster '$$cluster'"; \
			$(KIND) delete cluster --name "$$cluster" || true; \
			continue; \
		fi; \
		prefix="$$base-"; \
		case "$$cluster" in \
			"$$prefix"*) ;; \
			*) continue ;; \
		esac; \
		suffix="$${cluster#$$prefix}"; \
		case "$$suffix" in \
			''|*[!0-9]*) continue ;; \
			*) \
				echo "Deleting Kind cluster '$$cluster'"; \
				$(KIND) delete cluster --name "$$cluster" || true; \
				;; \
		esac; \
	done <<< "$$clusters"

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	"$(GOLANGCI_LINT)" run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	"$(GOLANGCI_LINT)" run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	"$(GOLANGCI_LINT)" config verify

.PHONY: vulncheck
vulncheck: govulncheck ## Run govulncheck to scan for known vulnerabilities (production code only).
	"$(GOVULNCHECK)" ./...

##@ Build

.PHONY: update-trusted-root
update-trusted-root: ## Download/update the embedded trusted_root.json file for keyless verification.
	@echo "Fetching latest trusted_root.json from Sigstore TUF repository..."
	@go run internal/security/fetch_trusted_root.go

.PHONY: build
build: verify-trusted-root manifests generate fmt vet ## Build manager binary (dispatcher for provisioner and controller).
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host. Usage: make run COMMAND=provisioner or make run COMMAND=controller
	@if [ -z "$(COMMAND)" ]; then \
		echo "Error: COMMAND is required. Use 'make run COMMAND=provisioner' or 'make run COMMAND=controller'"; \
		exit 1; \
	fi
	go run ./cmd/main.go $(COMMAND)

.PHONY: run-provisioner
run-provisioner: manifests generate fmt vet ## Run the provisioner controller from your host.
	go run ./cmd/main.go provisioner

.PHONY: run-controller
run-controller: manifests generate fmt vet ## Run the OpenBaoCluster controller from your host.
	go run ./cmd/main.go controller

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager (dispatcher binary for both provisioner and controller).
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: docker-build-init
docker-build-init: ## Build docker image with the bao-config-init helper.
	$(CONTAINER_TOOL) build -f Dockerfile.init -t ${IMG} .

.PHONY: docker-push-init
docker-push-init: ## Push docker image with the bao-config-init helper.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: docker-build-backup
docker-build-backup: ## Build docker image with the backup-executor helper.
	$(CONTAINER_TOOL) build -f Dockerfile.backup -t ${IMG} .

.PHONY: docker-build-upgrade
docker-build-upgrade: ## Build docker image with the upgrade executor helper.
	$(CONTAINER_TOOL) build -f Dockerfile.upgrade -t ${IMG} .

.PHONY: docker-push-backup
docker-push-backup: ## Push docker image with the backup-executor helper.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: docker-build-all

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
# Note: The built image contains the dispatcher binary that can run as either provisioner or controller.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager (dispatcher) for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name openbao-operator-builder
	$(CONTAINER_TOOL) buildx use openbao-operator-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm openbao-operator-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployments (provisioner and controller).
	@tmp="$$(mktemp -d)"; \
	trap 'rm -rf "$$tmp"' EXIT; \
	out="$(PWD)/dist/install.yaml"; \
	mkdir -p dist; \
	cp -R config "$$tmp/config"; \
	for f in "$$tmp/config/manager/controller.yaml" "$$tmp/config/manager/provisioner.yaml"; do \
		python3 -c 'import pathlib,re,sys; p=pathlib.Path(sys.argv[1]); v=sys.argv[2]; q=chr(34); s=p.read_text(encoding="utf-8"); s=re.sub(r"(\\n\\s*-\\s*name:\\s*OPERATOR_VERSION\\s*\\n\\s*value:\\s*)(\\\"[^\\\"]*\\\"|[^\\n#]+)", lambda m: m.group(1)+q+v+q, s, count=1); p.write_text(s, encoding="utf-8")' "$$f" "$(OPERATOR_VERSION)"; \
	done; \
	( cd "$$tmp/config/manager" && "$(KUSTOMIZE)" edit set image controller=${IMG} ); \
	"$(KUSTOMIZE)" build "$$tmp/config/default" > "$$out"

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

ifndef wait
  wait = true
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	@out="$$( "$(KUSTOMIZE)" build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | "$(KUBECTL)" apply -f -; else echo "No CRDs to install; skipping."; fi

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	@out="$$( "$(KUSTOMIZE)" build config/crd 2>/dev/null || true )"; \
	if [ -n "$$out" ]; then echo "$$out" | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) --wait=$(wait) -f -; else echo "No CRDs to delete; skipping."; fi

.PHONY: deploy
deploy: manifests kustomize ## Deploy both provisioner and controller to the K8s cluster specified in ~/.kube/config.
	@tmp="$$(mktemp -d)"; \
	trap 'rm -rf "$$tmp"' EXIT; \
	cp -R config "$$tmp/config"; \
	for f in "$$tmp/config/manager/controller.yaml" "$$tmp/config/manager/provisioner.yaml"; do \
		python3 -c 'import pathlib,re,sys; p=pathlib.Path(sys.argv[1]); v=sys.argv[2]; q=chr(34); s=p.read_text(encoding="utf-8"); s=re.sub(r"(\\n\\s*-\\s*name:\\s*OPERATOR_VERSION\\s*\\n\\s*value:\\s*)(\\\"[^\\\"]*\\\"|[^\\n#]+)", lambda m: m.group(1)+q+v+q, s, count=1); p.write_text(s, encoding="utf-8")' "$$f" "$(OPERATOR_VERSION)"; \
	done; \
	( cd "$$tmp/config/manager" && "$(KUSTOMIZE)" edit set image controller=${IMG} ); \
	"$(KUSTOMIZE)" build "$$tmp/config/default" | "$(KUBECTL)" apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy both provisioner and controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion. Call with wait=false to avoid waiting for finalizers.
	"$(KUSTOMIZE)" build config/default | "$(KUBECTL)" delete --ignore-not-found=$(ignore-not-found) --wait=$(wait) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p "$(LOCALBIN)"

## Tool Binaries
KUBECTL ?= kubectl
KIND ?= kind
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
GINKGO ?= $(LOCALBIN)/ginkgo
GOVULNCHECK ?= $(LOCALBIN)/govulncheck

## Tool Versions
KUSTOMIZE_VERSION ?= v5.7.1
CONTROLLER_TOOLS_VERSION ?= v0.19.0
GINKGO_VERSION ?= v2.22.0

#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell v='$(call gomodver,sigs.k8s.io/controller-runtime)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_VERSION manually (controller-runtime replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?([0-9]+)\.([0-9]+).*/release-\1.\2/')

#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell v='$(call gomodver,k8s.io/api)'; \
  [ -n "$$v" ] || { echo "Set ENVTEST_K8S_VERSION manually (k8s.io/api replace has no tag)" >&2; exit 1; }; \
  printf '%s\n' "$$v" | sed -E 's/^v?[0-9]+\.([0-9]+).*/1.\1/')

GOLANGCI_LINT_VERSION ?= v2.5.0
GOVULNCHECK_VERSION ?= v1.1.4
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@"$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: ginkgo
ginkgo: $(GINKGO) ## Download ginkgo CLI locally if necessary.
$(GINKGO): $(LOCALBIN)
	$(call go-install-tool,$(GINKGO),github.com/onsi/ginkgo/v2/ginkgo,$(GINKGO_VERSION))

.PHONY: govulncheck
govulncheck: $(GOVULNCHECK) ## Download govulncheck locally if necessary.
$(GOVULNCHECK): $(LOCALBIN)
	$(call go-install-tool,$(GOVULNCHECK),golang.org/x/vuln/cmd/govulncheck,$(GOVULNCHECK_VERSION))

.PHONY: security-scan
security-scan: ## Run Trivy security scans (filesystem and container image)
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
		--skip-files config/rbac/provisioner_delegate_clusterrole.yaml \
		--skip-files config/rbac/provisioner_minimal_role.yaml \
		--skip-files config/rbac/single_tenant_clusterrole.yaml \
		--skip-files dist/install.yaml \
		--skip-files charts/openbao-operator/templates/rbac/provisioner-clusterroles.yaml \
		.
	trivy image \
		--severity HIGH,CRITICAL \
		--ignore-unfixed \
		--exit-code 1 \
		--skip-version-check \
		${IMG}

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] && [ "$$(readlink -- "$(1)" 2>/dev/null)" = "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f "$(1)" ;\
GOBIN="$(LOCALBIN)" go install $${package} ;\
mv "$(LOCALBIN)/$$(basename "$(1)")" "$(1)-$(3)" ;\
} ;\
ln -sf "$$(realpath "$(1)-$(3)")" "$(1)"
endef

define gomodver
$(shell go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' $(1) 2>/dev/null)
endef
