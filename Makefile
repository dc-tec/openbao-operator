# Image URL to use all building/pushing image targets
IMG ?= controller:latest

# OPERATOR_VERSION is injected into the controller/provisioner Deployments.
# This is used by the controller to derive version-matched helper images.
OPERATOR_VERSION ?= 0.0.0

# INIT_IMG is the image for the config-init helper used as an init container
# in OpenBao pods to render the final config.hcl from the template.
# When running init-image targets, you can either set INIT_IMG explicitly:
#   make docker-build-init INIT_IMG=localhost:5000/openbao-init:dev
# or reuse IMG:
#   make docker-build-init IMG=localhost:5000/openbao-init:dev
INIT_IMG ?= openbao-init:latest

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

# REGISTRY and VERSION are used by docker-release to tag images consistently.
# Default is a local registry for development. Override for production:
#   make docker-release REGISTRY=ghcr.io/myorg VERSION=v1.2.3
REGISTRY ?= localhost:5000
VERSION ?= latest

# Image names for docker-release target (all share the same VERSION)
MANAGER_IMG ?= $(REGISTRY)/openbao-operator:$(VERSION)
INIT_IMG_RELEASE ?= $(REGISTRY)/openbao-init:$(VERSION)
BACKUP_IMG ?= $(REGISTRY)/openbao-backup:$(VERSION)
UPGRADE_IMG ?= $(REGISTRY)/openbao-upgrade:$(VERSION)

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# Use vendored dependencies for deterministic/reproducible Go builds.
GOFLAGS_VENDOR ?= -mod=vendor

include mk/general.mk
include mk/ci.mk
include mk/development.mk
include mk/build.mk
include mk/deploy.mk
include mk/dependencies.mk
