# Commands
GO := go
KUBECTL := kubectl

# Directories
RUNTIME_SRC_DIR := ./cmd/runtime-test
INSTALL_SRC_DIR := ./cmd/install-argocd

# CRD Paths
CRD_FILE := ./apis/v1alpha1/crd.yaml

# Targets
.PHONY: all generate-crds build-runtime build-install setup apply-crd run-install run-runtime run

all: run

generate-crds:
	@echo "+ Generating CRDs"
	@go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest
	@$(shell go env GOPATH)/bin/controller-gen crd paths="./apis/..." output:crd:stdout > $(CRD_FILE)

build-runtime:
	$(GO) build -o bin/runtime-test $(RUNTIME_SRC_DIR)

build-install:
	$(GO) build -o bin/install-argocd $(INSTALL_SRC_DIR)

setup:
	./hack/setup.sh

apply-crd:
	@echo "+ Applying CRD"
	@$(KUBECTL) apply -f $(CRD_FILE)

run-install: build-install
	@echo "+ Running install-argocd binary"
	@./bin/install-argocd

run-runtime: build-runtime
	@echo "+ Running runtime-test binary"
	@./bin/runtime-test

run: setup generate-crds apply-crd run-install run-runtime
	@echo "+ Completed setup, CRD generation, application, and binaries execution"

## Display help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build-runtime          Builds the runtime-test binary"
	@echo "  build-install          Builds the install-argocd binary"
	@echo "  setup                  Setup script before build"
	@echo "  generate-crds          Generates CRDs from struct"
	@echo "  apply-crd              Apply generated CRD to the Kubernetes cluster"
	@echo "  run-install            Run the install-argocd binary"
	@echo "  run-runtime            Run the runtime-test binary"
	@echo "  run          Setup, generate CRDs, apply them, run install-argocd, and run runtime-test"
	@echo "  help                   Display this help message"
