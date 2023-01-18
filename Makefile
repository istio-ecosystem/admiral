DOCKER_REPO?=admiralproj
IMAGE?=$(DOCKER_REPO)/admiral
DOCKER_USER?=aattuluri
KUSTOMIZE_VERSION?=4.5.5

DOCKERFILE?=Dockerfile.admiral

SHELL := /bin/bash
# Go parameters
GOCMD?=go
GOBUILD?=$(GOCMD) build
GOCLEAN?=$(GOCMD) clean
GOTEST?=$(GOCMD) test
GOGET?=$(GOCMD) get
GOBIN?=$(GOPATH)/bin
OUT?=./out/

BINARY_NAME?=$(OUT)admiral
BINARY_DARWIN?=$(BINARY_NAME)_darwin

#Protoc
PROTOC_VER=3.9.1
PROTOC_ZIP=protoc-$(PROTOC_VER)-osx-x86_64.zip


# ROOT_PACKAGE :: the package (relative to $GOPATH/src) that is the target for code generation
ROOT_PACKAGE=github.com/istio-ecosystem/admiral/admiral
# CUSTOM_RESOURCE_NAME :: the name of the custom resource that we're generating client code for
CUSTOM_RESOURCE_NAME=admiral
# CUSTOM_RESOURCE_VERSION :: the version of the resource
CUSTOM_RESOURCE_VERSION=v1

MAIN_PATH_ADMIRAL=./admiral/cmd/admiral/main.go
OPSYS:=$(shell $(GOCMD) env GOOS)

PATH:=$(GOBIN):$(PATH)


all: build test

build: build-linux

build-mac:
	$(GOBUILD) -o $(BINARY_DARWIN) -v $(MAIN_PATH_ADMIRAL)

test:
	$(GOTEST) -v `go list ./... | grep -v client` -coverprofile=c.out

clean:
	$(GOCLEAN)
	rm -fr $(OUT)

dep:
	$(DEP_PATH)dep ensure -v

setup:
	$(GOGET) -u github.com/golang/protobuf/protoc-gen-go@v1.3.2


gen-all: api-gen crd-gen

install-protoc-mac:
	curl -OL https://github.com/google/protobuf/releases/download/v$(PROTOC_VER)/$(PROTOC_ZIP)
	unzip -o $(PROTOC_ZIP) -d /usr/local bin/protoc
	rm -f $(PROTOC_ZIP)

api-gen:
	#make sure the protobuf matches the generation plugin
	$(GOCMD) generate ./...
	go install k8s.io/code-generator/cmd/deepcopy-gen

crd-gen:
	go get -d -u -fix k8s.io/code-generator@v0.17.3
	go get -d -u -fix k8s.io/apimachinery@v0.17.3
	go get -d -u -fix k8s.io/gengo
	$(GOPATH)/src/k8s.io/code-generator/generate-groups.sh all "$(ROOT_PACKAGE)/pkg/client" "$(ROOT_PACKAGE)/pkg/apis" "$(CUSTOM_RESOURCE_NAME):$(CUSTOM_RESOURCE_VERSION)"

# Cross compilation
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME) -v $(MAIN_PATH_ADMIRAL)

#set tag to `latest` if master branch and TAG is not set
#set tag to commit sha if TAG is not set and is not master branch
set-tag:
ifndef TAG
ifeq ($(BRANCH),master)
override TAG=latest
endif
endif
ifndef TAG
override TAG=$(SHA)
endif

docker-build: set-tag
    #NOTE: Assumes binary has already been built (admiral)
	docker build -t $(IMAGE):$(TAG) -f ./admiral/docker/$(DOCKERFILE) .

docker-publish:
ifndef DO_NOT_PUBLISH
	echo "$(DOCKER_PASS)" | docker login -u $(DOCKER_USER) --password-stdin
endif
ifeq ($(TAG),)
	echo "This is not a Tag/Release, skipping docker publish"
else
ifndef DO_NOT_PUBLISH
	docker push $(IMAGE):$(TAG)
endif
endif
#no tag set and its master branch, in this case publish `latest` tag
ifeq ($(TAG),)
ifeq ($(BRANCH),master)
	docker push $(IMAGE):latest
else
	echo "This is not master branch, skipping to publish 'latest' tag"
endif
endif

download-kustomize:
	curl -s -O -L https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize/v${KUSTOMIZE_VERSION}/kustomize_v${KUSTOMIZE_VERSION}_${OPSYS}_amd64.tar.gz
	tar xzf ./kustomize_v${KUSTOMIZE_VERSION}_${OPSYS}_amd64.tar.gz
	chmod u+x kustomize

gen-yaml:
	mkdir -p ./out/yaml
	mkdir -p ./out/scripts
	kustomize build ./install/admiral/overlays/demosinglecluster/ > ./out/yaml/demosinglecluster.yaml
	kustomize build ./install/admiralremote/base/ > ./out/yaml/remotecluster.yaml
	kustomize build ./install/sample/overlays/deployment > ./out/yaml/sample.yaml
	kustomize build ./install/sample/overlays/grpc > ./out/yaml/grpc.yaml
	kustomize build ./install/sample/overlays/rollout-canary > ./out/yaml/sample-greeting-rollout-canary.yaml
	kustomize build ./install/sample/overlays/rollout-bluegreen > ./out/yaml/sample-greeting-rollout-bluegreen.yaml
	kustomize build ./install/sample/overlays/remote > ./out/yaml/remotecluster_sample.yaml
	cp ./install/sample/sample_dep.yaml ./out/yaml/sample_dep.yaml
	cp ./install/sample/greeting_preview.yaml ./out/yaml/greeting_preview.yaml
	cp ./install/sample/gtp.yaml ./out/yaml/gtp.yaml
	cp ./install/sample/gtp_failover.yaml ./out/yaml/gtp_failover.yaml
	cp ./install/sample/gtp_topology.yaml ./out/yaml/gtp_topology.yaml
	cp ./install/sample/grpc-client.yaml ./out/yaml/grpc-client.yaml
	cp ./install/prometheus/prometheus.yaml ./out/yaml/prometheus.yaml
	cp ./install/scripts/*.sh ./out/scripts/
