KUBEBUILDER_VERSION = 2.3.1
export KUBEBUILDER_ASSETS = ${PWD}/cache/kubebuilder_${KUBEBUILDER_VERSION}/bin
CONTROLLER_GEN_VERSION = 0.2.5
CONTROLLER_GEN=${PWD}/cache/controller-gen_${CONTROLLER_GEN_VERSION}/controller-gen
LINT_VERSION = 1.28.3
KUBEVAL_VERSION= 0.15.0
KUBEVAL_KUBE_VERSION=1.18.1
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
		curl -L https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${KUBEBUILDER_VERSION}/kubebuilder_${KUBEBUILDER_VERSION}_$(shell go env GOOS)_$(shell go env GOARCH).tar.gz | tar --strip-components=1 -xz -C ./cache/kubebuilder_${KUBEBUILDER_VERSION}; \
	fi

.PHONY: kustomize
kustomize: cache/bin/kustomize

cache/bin/kustomize: cache/bin
	@rm -f cache/bin/kustomize
	cd cache/bin && \
		set -o pipefail && \
		for (( i = 0; i < 5; i++ )); do \
			curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash; \
			if [[ "$$(which kustomize)" =~ cache/bin/kustomize ]]; then \
				break; \
			fi \
		done
	[[ "$$(which kustomize)" =~ cache/bin/kustomize ]]

.PHONY: test-unit
test-unit: generate manifests kubebuilder
	go test -race -short -coverprofile cover.out ./...

.PHONY: test
test: generate manifests kubebuilder
	go test -race -coverprofile cover.out ./...

.PHONY: coverage-unit
coverage-unit: test-unit
	go install github.com/mattn/goveralls@v0.0.11
	$(GOBIN)/goveralls -coverprofile="cover.out" -service=travis-ci

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
	kubectl delete secret/ibmcloud-operator-secret configmap/ibmcloud-operator-defaults

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
	@if ! which shellcheck; then \
		set -ex; curl -fsSL https://github.com/koalaman/shellcheck/releases/download/stable/shellcheck-stable.$$(uname).x86_64.tar.xz | tar -xJv --strip-components=1 shellcheck-stable/shellcheck; \
		mv shellcheck $(shell go env GOPATH)/bin/shellcheck; chmod +x $(shell go env GOPATH)/bin/shellcheck; \
	fi

.PHONY: lint
lint: lint-deps
	golangci-lint run
	find . -name '*.*sh' | xargs shellcheck --color
	go list -json -m all | docker run --rm -i sonatypecommunity/nancy:latest sleuth

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
release-prep: kustomize manifests out 
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default --output out/
	ulimit -n 1000 && go run ./internal/cmd/genolm --version ${RELEASE_VERSION}

.PHONY: release-operatorhub
release-operatorhub:
	go run ./internal/cmd/release \
		-version "${RELEASE_VERSION}" \
		-csv "out/ibmcloud_operator.v${RELEASE_VERSION}.clusterserviceversion.yaml" \
		-package out/ibmcloud-operator.package.yaml \
		-crd-glob 'out/apiextensions.k8s.io_*_customresourcedefinition_*.ibmcloud.ibm.com.yaml' \
		-draft=$${RELEASE_DRAFT:-false} \
		-fork-org "$${RELEASE_FORK_ORG}" \
		-gh-token "$${RELEASE_GH_TOKEN}" \
		-signoff-name "$${RELEASE_GIT_NAME}" \
		-signoff-email "$${RELEASE_GIT_EMAIL}"

.PHONY: release
release: release-prep docker-push release-operatorhub

# Validates release artifacts.
# TODO add validation for operator-courier. Currently hitting WAY too many issues with Travis CI and Python deps.
.PHONY: validate-release
validate-release: kubeval release-prep docker-build
	kubeval -d out --kubernetes-version "${KUBEVAL_KUBE_VERSION}" --ignored-filename-patterns package.yaml --ignore-missing-schemas

.PHONY: operator-courier
operator-courier:
	@if ! which operator-courier; then \
		pip3 install operator-courier; \
	fi

.PHONY: verify-operator-meta
verify-operator-meta: release-prep operator-courier
	operator-courier verify --ui_validate_io out/

.PHONY: operator-push-test
operator-push-test: IMG = quay.io/${QUAY_NAMESPACE}/${QUAY_REPO}:${RELEASE_VERSION}
operator-push-test: verify-operator-meta docker-build
	# Example values:
	#
	# QUAY_NAMESPACE=myuser
	# QUAY_REPO=ibmcloud-operator-image
	# QUAY_APP=ibmcloud-operator  NOTE: Must have a repository AND a quay "application". They aren't the same thing.
	# QUAY_USER=myuser+mybot      NOTE: Bot users are best, so you can manage permissions better.
	# QUAY_TOKEN=abcdef1234567
	@for v in "${QUAY_NAMESPACE}" "${QUAY_APP}" "${QUAY_REPO}" "${RELEASE_VERSION}" "${QUAY_USER}" "${QUAY_TOKEN}"; do \
		if [[ -z "$$v" ]]; then \
			echo 'Not all Quay variables set. See the make target for details.'; \
			exit 1; \
		fi; \
	done
	docker login -u="${QUAY_USER}" -p="${QUAY_TOKEN}" quay.io
	docker push "${IMG}"
	operator-courier push ./out "${QUAY_NAMESPACE}" "${QUAY_APP}" "${RELEASE_VERSION}" "Basic $$(printf "${QUAY_USER}:${QUAY_TOKEN}" | base64)"

.PHONY: kubeval
kubeval: cache/bin
	@if [[ ! -f cache/bin/kubeval ]]; then \
		set -ex -o pipefail; \
		curl -sL https://github.com/instrumenta/kubeval/releases/download/${KUBEVAL_VERSION}/kubeval-$$(uname)-amd64.tar.gz | tar -xz -C cache/bin; \
	fi
