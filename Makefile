# Image URL to use all building/pushing image targets
IMG ?= cloudoperators/ibmcloud-operator
GOFILES = $(shell find . -type f -name '*.go' -not -path "./vendor/*")

deps:
	go get golang.org/x/lint/golint
	go get -u github.com/apg/patter
	go get -u github.com/wadey/gocovmerge
	go get -u github.com/alecthomas/gometalinter
	gometalinter --install
	pip install --user PyYAML

all: test manager

# Run tests
test: generate fmt vet manifests
	go test ./pkg/... ./cmd/... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager github.com/ibm/cloud-operators/cmd/manager

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet
	go run ./cmd/manager/main.go

# Install CRDs into a cluster
install: manifests
	kubectl apply -f config/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	kubectl apply -f config/crds
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests:
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all
	hack/crd-fix.sh

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

# Generate code
generate:
	go generate ./pkg/... ./cmd/...
	hack/update-codegen.sh

# Build the docker image
docker-build: check-tag
	git rev-parse --short HEAD > git-rev
	docker build . -t ${IMG}:${TAG}
	rm git-rev
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}:${TAG}"'@' ./config/default/manager_image_patch.yaml

# Push the docker image
docker-push: check-tag
	echo "${DOCKER_PASSWORD}" | docker login -u "${DOCKER_USERNAME}" --password-stdin
	docker push ${IMG}:${TAG}

# Run the operator-sdk scorecard on latest release
scorecard:
	hack/operator-scorecard.sh 

# make an initial release for olm and releases
release: check-tag
	python hack/package.py v${TAG}

# make an initial release for olm and releases
release-update: check-tag
	python hack/package.py v${TAG} --is_update

# Push OLM metadata to private Quay registry
push-olm: check-tag check-quaytoken check-quayns
	operator-courier push olm/v${TAG} ${QUAY_NS} ibmcloud-operator ${TAG} "${QUAY_TOKEN}"
	@echo Remember to make https://quay.io/application/${QUAY_NS}/ibmcloud-operator public

.PHONY: lintall
lintall: fmt lint vet

lint:
	golint -set_exit_status=true pkg/

check-tag:
ifndef TAG
	$(error TAG is undefined! Please set TAG to the latest release tag, using the format x.y.z e.g. export TAG=0.1.1 ) 
endif

check-quayns:
ifndef QUAY_NS
	$(error QUAY_NS is undefined!) 
endif

check-quaytoken:
ifndef QUAY_TOKEN
	$(error QUAY_TOKEN is undefined!) 
endif