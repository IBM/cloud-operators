KUBEBUILDER_VERSION = 2.3.1
export KUBEBUILDER_ASSETS = ${PWD}/cache/kubebuilder_${KUBEBUILDER_VERSION}/bin
CONTROLLER_GEN_VERSION = 0.2.5
CONTROLLER_GEN=${PWD}/cache/controller-gen_${CONTROLLER_GEN_VERSION}/controller-gen
LINT_VERSION = 1.28.3
# Set PATH to pick up cached tools. The additional 'sed' is required for cross-platform support of quoting the args to 'env'
SHELL := /usr/bin/env PATH=$(shell echo ${PWD}/cache/bin:${KUBEBUILDER_ASSETS}:${PATH} | sed 's/ /\\ /g') bash

# Version to create release. Value is set in .travis.yml's release job
RELEASE_VERSION ?= 0.0.0
# Image URL to use all building/pushing image targets
IMG ?= cloudoperators/ibmcloud-operator:${RELEASE_VERSION}
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

.PHONY: all
all: manager

# Prints exported env vars for use in ad hoc scripts, like 'go test ./... -run TestMyTest'
.PHONY: env
env:
	@env | grep KUBEBUILDER

cache:
	mkdir -p cache

cache/bin:
	mkdir -p cache/bin

.PHONY: clean
clean:
	rm -rf cache out

# Ensures kubebuilder is installed into the cache. Run `make kubebuilder CMD="--help"` to run kubebuilder with a custom command.
.PHONY: kubebuilder
kubebuilder: cache/kubebuilder_${KUBEBUILDER_VERSION}/bin
	@if [[ -n "${CMD}" ]]; then \
		set -ex; \
		kubebuilder ${CMD}; \
		find . -name '*.go' | xargs sed -i '' -e "s/YEAR/$(shell date +%Y)/"; \
	fi

cache/kubebuilder_${KUBEBUILDER_VERSION}/bin: cache
	@if [[ ! -d cache/kubebuilder_${KUBEBUILDER_VERSION}/bin ]]; then \
		rm -rf cache/kubebuilder_${KUBEBUILDER_VERSION}; \
		mkdir -p cache/kubebuilder_${KUBEBUILDER_VERSION}; \
		set -o pipefail; \
		curl -L https://go.kubebuilder.io/dl/${KUBEBUILDER_VERSION}/$(shell go env GOOS)/$(shell go env GOARCH) | tar --strip-components=1 -xz -C ./cache/kubebuilder_${KUBEBUILDER_VERSION}; \
	fi

.PHONY: kustomize
kustomize: cache/bin/kustomize

cache/bin/kustomize: cache/bin
	@rm -f cache/bin/kustomize
	cd cache/bin && \
		set -o pipefail && \
		curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash
	[[ "$$(which kustomize)" =~ cache/bin/kustomize ]]

.PHONY: test-fast
test-fast: generate manifests kubebuilder
	go test -short -coverprofile cover.out ./...

.PHONY: test
test: generate manifests kubebuilder
	go test -race -coverprofile cover.out ./...

.PHONY: test-e2e
test-e2e:
	exit 0  # not implemented

.PHONY: coverage
coverage: test
	bash <(curl -s https://codecov.io/bash)

# Build manager binary
.PHONY: manager
manager: generate lint-fix
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
.PHONY: run
run: generate lint-fix manifests
	go run ./main.go

# Install CRDs into a cluster
.PHONY: install
install: manifests kustomize
	go run ./internal/cmd/firstsetup # Install ICO secret & configmap
	kustomize build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
.PHONY: uninstall
uninstall: manifests kustomize
	kustomize build config/crd | kubectl delete -f -
	kubectl delete secret/secret-ibm-cloud-operator configmap/config-ibm-cloud-operator

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
.PHONY: deploy
deploy: manifests kustomize
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	go run ./internal/cmd/fixcrd ./config/crd/bases/*.yaml

.PHONY: lint-deps
lint-deps:
	@if ! which golangci-lint >/dev/null || [[ "$$(golangci-lint --version)" != *${LINT_VERSION}* ]]; then \
		set -o pipefail; \
		curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin v${LINT_VERSION}; \
	fi

.PHONY: lint
lint: lint-deps
	golangci-lint run

.PHONY: lint-fix
lint-fix: lint-deps
	golangci-lint run --fix

.PHONY: generate
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt",year=$(shell date +%Y) paths="./..."

.PHONY: docker-build
docker-build:
	docker build . -t ${IMG}

.PHONY: docker-push
docker-push: docker-build
	if [[ ${RELEASE_VERSION} == 0.0.0 ]]; then \
		echo Refusing to push development image version 0.0.0; \
	else \
		if [[ -n "$$DOCKER_USERNAME" ]]; then \
			echo "$$DOCKER_PASSWORD" | docker login -u "$$DOCKER_USERNAME" --password-stdin; \
		fi; \
		docker push ${IMG}; \
	fi

# find or download controller-gen
# download controller-gen if necessary
.PHONY: controller-gen
controller-gen: cache/controller-gen_${CONTROLLER_GEN_VERSION}

cache/controller-gen_${CONTROLLER_GEN_VERSION}: cache
	@if [[ ! -f cache/controller-gen_${CONTROLLER_GEN_VERSION}/controller-gen ]]; then \
		set -ex ;\
		CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
		trap "rm -rf $$CONTROLLER_GEN_TMP_DIR" EXIT ;\
		cd $$CONTROLLER_GEN_TMP_DIR ;\
		go mod init tmp ;\
		GOBIN=${PWD}/cache/controller-gen_${CONTROLLER_GEN_VERSION} go get sigs.k8s.io/controller-tools/cmd/controller-gen@v${CONTROLLER_GEN_VERSION} ;\
	fi

out:
	mkdir -p out

# Prepares Kubernetes yaml files for release. Useful for testing against your own cluster.
.PHONY: release-prep
release-prep: kustomize out 
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default --output out/
	go run ./internal/cmd/genolm --version ${RELEASE_VERSION}

.PHONY: release
release: release-prep docker-push
