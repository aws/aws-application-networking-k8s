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
VERSION ?= $(if $(RELEASE_VERSION),$(RELEASE_VERSION),$(shell git tag --sort=v:refname | tail -1))
ECRIMAGES ?=public.ecr.aws/aws-application-networking-k8s/aws-gateway-controller:${VERSION}

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.32

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
presubmit: vet manifest lint test ## Run all commands before submitting code

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
	cd test && go mod tidy && go vet ./...


.PHONY: lint
lint: ## Run the golangci-lint only in local machine
	if command -v golangci-lint &> /dev/null; then \
		echo "Running golangci-lint"; \
		golangci-lint run; \
	else \
		echo "Error: golangci-lint is not installed. Please run the 'make setup'"; \
		exit 1; \
	fi \


.PHONY: test
test: ## Run tests.
	go test ./pkg/... -coverprofile coverage.out

.PHONY: setup
setup:
	./scripts/setup.sh

##@ Deployment

.PHONY: docker-build
docker-build: test ## Build docker image with the manager.
	sudo docker build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

# also generates a placeholder cert for the webhook - this cert is not intended to be valid
.PHONY: build-deploy
build-deploy: ## Create a deployment file that can be applied with `kubectl apply -f deploy.yaml`
	cd config/manager && kustomize edit set image controller=${ECRIMAGES}
	kustomize build config/default > deploy.yaml

.PHONY: manifest
manifest: ## Generate CRD manifest
	go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.18.0 object paths=./pkg/apis/...
	go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.18.0 crd paths=./pkg/apis/... output:crd:artifacts:config=config/crds/bases
	go run k8s.io/code-generator/cmd/register-gen@v0.34.1 --logtostderr ./pkg/apis/applicationnetworking/v1alpha1 --go-header-file hack/boilerplate.go.txt
	cp config/crds/bases/application-networking.k8s.aws* helm/crds

e2e-test-namespace := "e2e-test"

.PHONY: e2e-test
e2e-test: ## Run e2e tests against cluster pointed to by ~/.kube/config
	@kubectl create namespace $(e2e-test-namespace) > /dev/null 2>&1 || true # ignore already exists error
	LOG_LEVEL=debug
	cd test && go test \
		-p 1 \
		-count 1 \
		-timeout 120m \
		-v \
		./suites/integration/... \
		--ginkgo.focus="${FOCUS}" \
		--ginkgo.skip="${SKIP}" \
		--ginkgo.timeout=120m \
		--ginkgo.v

.SILENT:
.PHONY: e2e-clean
e2e-clean: ## Delete eks resources created in the e2e test namespace
	@echo -n "Cleaning up e2e tests... "
	-@kubectl delete namespace $(e2e-test-namespace)
	@kubectl create namespace $(e2e-test-namespace)
	@echo "Done!"

conformance-test-namespace := "conformance-test"
conformance-helm-release := "gateway-api-controller"
conformance-helm-chart := "oci://public.ecr.aws/aws-application-networking-k8s/aws-gateway-controller-chart"
conformance-helm-namespace := "aws-application-networking-system"

.PHONY: conformance-test
conformance-test: ## Run conformance tests. Set INSTALL_CONTROLLER=true to install/uninstall controller via Helm.
ifndef GATEWAY_API_VERSION
	$(error GATEWAY_API_VERSION is required (e.g., GATEWAY_API_VERSION=v1.4.0))
endif
ifndef CONTROLLER_VERSION
	$(error CONTROLLER_VERSION is required (e.g., CONTROLLER_VERSION=v2.1.0))
endif
	@rm -rf gateway-api
	@echo "Cloning gateway-api at $(GATEWAY_API_VERSION)..."
	git clone --branch $(GATEWAY_API_VERSION) --depth 1 \
		https://github.com/kubernetes-sigs/gateway-api.git gateway-api
	cd gateway-api && git apply ../test/conformance/patches/001-gateway-address.patch
	@echo "Installing Gateway API CRDs at $(GATEWAY_API_VERSION)..."
	-@kubectl delete validatingadmissionpolicybinding safe-upgrades.gateway.networking.k8s.io 2>/dev/null
	-@kubectl delete validatingadmissionpolicy safe-upgrades.gateway.networking.k8s.io 2>/dev/null
	-@kubectl get crds -o name | grep gateway.networking.k8s.io | xargs -r kubectl delete 2>/dev/null
	-@kubectl get crds -o name | grep gateway.networking.x-k8s.io | xargs -r kubectl delete 2>/dev/null
	kubectl apply --server-side --force-conflicts -f https://github.com/kubernetes-sigs/gateway-api/releases/download/$(GATEWAY_API_VERSION)/experimental-install.yaml
ifdef INSTALL_CONTROLLER
	@echo "Installing controller $(CONTROLLER_VERSION) via Helm..."
	helm install $(conformance-helm-release) $(conformance-helm-chart) \
		--version $(CONTROLLER_VERSION) \
		--namespace $(conformance-helm-namespace) \
		--set=serviceAccount.create=false \
		--set=defaultServiceNetwork=conformance-test \
		--set=enableServiceNetworkOverride=true \
		--set=log.level=debug \
		--wait
endif
	kubectl apply -f files/controller-installation/gatewayclass.yaml
	@GO_VER=$$(grep '^go ' gateway-api/go.mod | awk '{print $$2}'); \
		echo "Building conformance test binary (go $$GO_VER)..."; \
		cp test/conformance/conformance_test.go gateway-api/conformance/conformance_test.go && \
		cp test/conformance/skip-tests.yaml gateway-api/conformance/skip-tests.yaml && \
		cd gateway-api/conformance && \
		CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOTOOLCHAIN=go$${GO_VER} GOPROXY=direct go test -c -o ../../test/conformance/conformance.test .
	@kubectl create namespace $(conformance-test-namespace) > /dev/null 2>&1 || true
	@kubectl delete pod conformance-runner -n $(conformance-test-namespace) --force 2>/dev/null || true
	kubectl run conformance-runner -n $(conformance-test-namespace) \
		--image=public.ecr.aws/docker/library/alpine:3.19 \
		--command -- sleep infinity
	kubectl wait --for=condition=Ready pod/conformance-runner \
		-n $(conformance-test-namespace) --timeout=60s
	@kubectl create clusterrolebinding conformance-runner-admin \
		--clusterrole=cluster-admin \
		--serviceaccount=$(conformance-test-namespace):default 2>/dev/null || true
	kubectl cp test/conformance/conformance.test $(conformance-test-namespace)/conformance-runner:/conformance.test
	kubectl cp test/conformance/skip-tests.yaml $(conformance-test-namespace)/conformance-runner:/skip-tests.yaml
	kubectl exec conformance-runner -n $(conformance-test-namespace) -- chmod +x /conformance.test
	@kubectl exec conformance-runner -n $(conformance-test-namespace) -- \
		env CONTROLLER_VERSION=$(CONTROLLER_VERSION) REPORT_OUTPUT=/report.yaml \
		/conformance.test \
		-test.v \
		-test.timeout 120m \
		-test.run TestConformance \
		|| true
	@echo "Copying conformance report..."
	-@kubectl cp $(conformance-test-namespace)/conformance-runner:/report.yaml ./conformance-report.yaml
	@echo "Report saved to conformance-report.yaml"
	@$(MAKE) conformance-clean

.PHONY: conformance-clean
conformance-clean: ## Delete resources created by conformance tests
	@echo -n "Cleaning up conformance tests... "
	-@kubectl delete pod conformance-runner -n $(conformance-test-namespace) --force
	-@kubectl delete clusterrolebinding conformance-runner-admin
	-@kubectl delete namespace $(conformance-test-namespace)
	-@helm uninstall $(conformance-helm-release) --namespace $(conformance-helm-namespace) 2>/dev/null
	-@rm -rf gateway-api
	-@rm -f test/conformance/conformance.test
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

e2e-webhook-namespace := "webhook-e2e-test"

# NB webhook tests can only run if the controller is deployed to the cluster
.PHONY: webhook-e2e-test
webhook-e2e-test:
	@kubectl create namespace $(e2e-webhook-namespace) > /dev/null 2>&1 || true # ignore already exists error
	LOG_LEVEL=debug
	cd test && go test \
		-p 1 \
		-count 1 \
		-timeout 10m \
		-v \
		./suites/webhook/... \
		--ginkgo.focus="${FOCUS}" \
		--ginkgo.skip="${SKIP}" \
		--ginkgo.v