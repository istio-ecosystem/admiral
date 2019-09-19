DOCKER_REPO=admiralproj
IMAGE=$(DOCKER_REPO)/admiral
TAG=latest
DOCKER_USER=aattuluri

SHELL := /bin/bash
# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOBIN=$(GOPATH)/bin
OUT=./out/

BINARY_NAME=$(OUT)admiral
BINARY_DARWIN=$(BINARY_NAME)_darwin

#Protoc
PROTOC_VER=3.9.1
PROTOC_ZIP=protoc-$(PROTOC_VER)-osx-x86_64.zip


# ROOT_PACKAGE :: the package (relative to $GOPATH/src) that is the target for code generation
ROOT_PACKAGE=github.com/admiral/admiral
# CUSTOM_RESOURCE_NAME :: the name of the custom resource that we're generating client code for
CUSTOM_RESOURCE_NAME=admiral
# CUSTOM_RESOURCE_VERSION :: the version of the resource
CUSTOM_RESOURCE_VERSION=v1

MAIN_PATH_ADMIRAL=./admiral/cmd/admiral/main.go


PATH:=$(GOBIN):$(PATH)


all: build test

build: build-linux

build-mac:
	$(GOBUILD) -o $(BINARY_DARWIN) -v $(MAIN_PATH_ADMIRAL)

test:
	$(GOTEST) ./...

clean:
	$(GOCLEAN)
	rm -fr $(OUT)

dep:
	$(DEP_PATH)dep ensure -v

setup:
	$(GOGET) -u github.com/golang/protobuf/protoc-gen-go


gen-all: api-gen crd-gen

install-protoc-mac:
	curl -OL https://github.com/google/protobuf/releases/download/v$(PROTOC_VER)/$(PROTOC_ZIP)
	unzip -o $(PROTOC_ZIP) -d /usr/local bin/protoc
	rm -f $(PROTOC_ZIP)

api-gen:
	#make sure the protobuf matches the generation plugin
	go install github.com/golang/protobuf/protoc-gen-go
	$(GOCMD) generate ./...
	go install k8s.io/code-generator/cmd/deepcopy-gen
	$(GOBIN)/deepcopy-gen --input-dirs ./admiral/pkg/apis/admiral/model --bounding-dirs ./admiral/pkg/apis/admiral/model -O zz_generated.deepcopy -o $(GOPATH)/src

crd-gen:
	#go get -d -u -fix k8s.io/code-generator
	#go get -d -u -fix k8s.io/apimachinery
	#go get -d -u -fix k8s.io/gengo
	$(GOPATH)/src/k8s.io/code-generator/generate-groups.sh all "$(ROOT_PACKAGE)/pkg/client" "$(ROOT_PACKAGE)/pkg/apis" "$(CUSTOM_RESOURCE_NAME):$(CUSTOM_RESOURCE_VERSION)"

# Cross compilation
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_NAME) -v $(MAIN_PATH_ADMIRAL)


docker-build:
	#NOTE: Assumes binary has already been built (admiral)
	docker build -t $(IMAGE):$(TAG) -f ./admiral/docker/Dockerfile.admiral .

docker-push:
	echo "$(DOCKER_PASS)" | docker login -u $(DOCKER_USER) --password-stdin
	docker push $(IMAGE):$(TAG)