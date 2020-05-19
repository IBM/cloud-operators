# Image URL to use all building/pushing image targets
IMG ?= cloudoperators/ibmcloud-operator

.PHONY: deps
deps:
	go get golang.org/x/lint/golint
	go get -u github.com/apg/patter
	go get -u github.com/wadey/gocovmerge
	go get -u github.com/alecthomas/gometalinter
	gometalinter --install
	pip install --user PyYAML

.PHONY: all
all: test manager

.PHONY: test
test: generate fmt vet manifests
	go test ./pkg/... ./cmd/... -coverprofile=cover.out -covermode=atomic

# Build manager binary
.PHONY: manager
manager: generate fmt vet
	go build -o bin/manager github.com/ibm/cloud-operators/cmd/manager

# Run against the configured Kubernetes cluster in ~/.kube/config
.PHONY: run
run: generate fmt vet
	go run ./cmd/manager/main.go

# Install CRDs into a cluster
.PHONY: install
install: manifests
	kubectl apply -f config/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
.PHONY: deploy
deploy: manifests
	kubectl apply -f config/crds
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests:
	go run vendor/sigs.k8s.io/controller-tools/cmd/controller-gen/main.go all
	hack/crd-fix.sh

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

# Generate code
.PHONY: generate
generate:
	go generate ./pkg/... ./cmd/...
	hack/update-codegen.sh

.PHONY: docker-build
docker-build: check-tag
	git rev-parse --short HEAD > git-rev
	docker build . -t ${IMG}:${TAG}
	rm git-rev
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}:${TAG}"'@' ./config/default/manager_image_patch.yaml

.PHONY: docker-push
docker-push: check-tag
	echo "${DOCKER_PASSWORD}" | docker login -u "${DOCKER_USERNAME}" --password-stdin
	docker push ${IMG}:${TAG}

# Run the operator-sdk scorecard on latest release
.PHONY: scorecard
scorecard:
	hack/operator-scorecard.sh 

# make an initial release for olm and releases
.PHONY: release
release: check-tag
	python hack/package.py v${TAG}

# make an initial release for olm and releases
.PHONY: release-update
release-update: check-tag
	python hack/package.py v${TAG} --is_update

# Push OLM metadata to private Quay registry
# operator-courier push olm/${TAG} ${QUAY_NS} ibmcloud-operator ${TAG} "${QUAY_TOKEN}"
.PHONY: push-olm
push-olm: check-tag check-quaytoken check-quayns
	operator-courier push olm ${QUAY_NS} ibmcloud-operator ${TAG} "${QUAY_TOKEN}"
	@echo Remember to make https://quay.io/application/${QUAY_NS}/ibmcloud-operator public

.PHONY: lintall
lintall: fmt lint vet

.PHONY: lint
lint:
	golint -set_exit_status=true pkg/

.PHONY: check-tag
check-tag:
ifndef TAG
	$(error TAG is undefined! Please set TAG to the latest release tag, using the format x.y.z e.g. export TAG=0.1.1 ) 
endif

.PHONY: check-quayns
check-quayns:
ifndef QUAY_NS
	$(error QUAY_NS is undefined!) 
endif

.PHONY: check-quaytoken
check-quaytoken:
ifndef QUAY_TOKEN
	$(error QUAY_TOKEN is undefined!) 
endif
