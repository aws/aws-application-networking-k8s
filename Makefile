# For all targets except `help` and default target (which is also `help`), export environment variables
ifneq (,$(filter-out help,$(MAKECMDGOALS)))
	export KUBEBUILDER_ASSETS ?= ${HOME}/.kubebuilder/bin
	export CLUSTER_NAME ?= $(shell kubectl config view --minify -o jsonpath='{.clusters[].name}' | rev | cut -d"/" -f1 | rev | cut -d"." -f1)
	export CLUSTER_VPC_ID ?= $(shell aws eks describe-cluster --name $(CLUSTER_NAME) | jq -r ".cluster.resourcesVpcConfig.vpcId")
	export AWS_ACCOUNT_ID ?= $(shell aws sts get-caller-identity --query Account --output text)
	export REGION ?= $(shell aws configure get region)
endif

# Image URL to use all building/pushing image targets
IMG ?= controller:latest
VERSION ?= $(shell git tag --sort=v:refname | tail -1)
ECRIMAGES ?=public.ecr.aws/aws-application-networking-k8s/aws-gateway-controller:${VERSION}

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.22

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
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

##@ Local Development

.PHONY: run
run: ## Run in development mode
	DEV_MODE=1 LOG_LEVEL=debug go run cmd/aws-application-networking-k8s/main.go


.PHONY: presubmit
presubmit: manifest vet lint test ## Run all commands before submitting code

.PHONY: vet
vet: ## Vet the code and dependencies
	go mod tidy
	go generate ./...
	go vet ./...
	go fmt ./...
	@git diff --quiet ||\
		{ echo "New file modification detected in the Git working tree. Please check in before commit."; git --no-pager diff --name-only | uniq | awk '{print "  - " $$0}'; \
		if [ "${CI}" = true ]; then\
			exit 1;\
		fi;}


.PHONY: lint
lint: ## Run the golangci-lint only in local machine
	if command -v golangci-lint &> /dev/null; then \
		echo "Running golangci-lint"; \
		golangci-lint run; \
	else \
		echo "Error: golangci-lint is not installed. Please run the 'make toolchain'"; \
		exit 1; \
	fi \


.PHONY: test
test: ## Run tests.
	go test ./pkg/... -coverprofile coverage.out

.PHONY: toolchain
toolchain: ## Install developer toolchain
	./hack/toolchain.sh

##@ Deployment

.PHONY: docker-build
docker-build: test ## Build docker image with the manager.
	sudo docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

.PHONY: build-deploy
build-deploy: ## Create a deployment file that can be applied with `kubectl apply -f deploy.yaml`
	cd config/manager && kustomize edit set image controller=${ECRIMAGES}
	kustomize build config/default > deploy.yaml

.PHONY: manifest
manifest: ## Generate CRD manifest
	go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.13.0 object paths=./pkg/apis/...
	go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.13.0 crd paths=./pkg/apis/... output:crd:artifacts:config=config/crds/bases
	go run k8s.io/code-generator/cmd/register-gen@v0.28.0 --input-dirs ./pkg/apis/applicationnetworking/v1alpha1 --output-base ./ --go-header-file hack/boilerplate.go.txt
	cp config/crds/bases/application-networking.k8s.aws* helm/crds

e2e-test-namespace := "e2e-test"

.PHONY: e2e-test
e2e-test: ## Run e2e tests against cluster pointed to by ~/.kube/config
	@kubectl create namespace $(e2e-test-namespace) > /dev/null 2>&1 || true # ignore already exists error
	LOG_LEVEL=debug
	cd test && go test \
		-p 1 \
		-count 1 \
		-timeout 90m \
		-v \
		./suites/... \
		--ginkgo.focus="${FOCUS}" \
		--ginkgo.timeout=90m \
		--ginkgo.v

.SILENT:
.PHONY: e2e-clean
e2e-clean: ## Delete eks resources created in the e2e test namespace
	@echo -n "Cleaning up e2e tests... "
	@kubectl delete namespace $(e2e-test-namespace) > /dev/null 2>&1
	@kubectl create namespace $(e2e-test-namespace) > /dev/null 2>&1
	@echo "Done!"

.PHONY: api-reference
api-reference: ## Update documentation in docs/api-reference.md
	@cd docgen && \
	gen-crd-api-reference-docs -config config.json -api-dir "../pkg/apis/applicationnetworking/v1alpha1/" -out-file docs.html && \
	cat api-reference-base.md docs.html > ../docs/api-reference.md

.PHONY: docs
docs:
	mkdir -p site
	mkdocs build
