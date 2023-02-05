COMMIT = $(shell git log --pretty=format:'%h' -n 1)
VERSION = $(shell git describe --abbrev=0 --exact-match || echo development)
PROJECT = "astrolavos"
USER = $(shell id -u)
GROUP = $(shell id -g)
GOBUILD_OPTS = -ldflags="-s -w -X main.Version=${VERSION} -X main.CommitHash=${COMMIT}"
GO_IMAGE = "golang:1.19-alpine"
GO_IMAGE_CI = "golangci/golangci-lint:v1.50.1"
DISTROLESS_IMAGE = "gcr.io/distroless/static:nonroot"
IMAGE_TAG_BASE ?= "ghcr.io/dntosas/${PROJECT}"

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint against code.
	@docker run --rm --volume "${PWD}:/app" -w /app "${GO_IMAGE_CI}" golangci-lint run --enable revive,gofmt,exportloopref --exclude-use-default=false --modules-download-mode=vendor --build-tags integration

.PHONY: test
test:
	go test -mod=vendor `go list ./... ` -race -coverprofile cover.out

.PHONY: envtest
envtest: ## Run go tests against code.
		KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test -v -mod=vendor `go list ./...` -coverprofile cover.out

.PHONY: ci
ci: fmt vet lint test ## Run go fmt/vet/lint/tests against the code.

.PHONY: e2e
e2e:
	cd tests && go mod vendor && go test -mod=vendor -v .

.PHONY: modsync
modsync: ## Run go mod tidy && vendor.
	go mod tidy && go mod vendor

.PHONY: helm-docs
helm-docs:
	docker run --rm --volume "${PWD}/charts/astrolavos:/helm-docs" -u ${USER} "jnorwood/helm-docs:v1.11.0"

##@ Build

.PHONY: build
build: ## Build astrolavos
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -mod=vendor ${GOBUILD_OPTS} -o bin/${PROJECT} .

build-local: ## Build astrolavos for local use
	CGO_ENABLED=0 go build -a -mod=vendor ${GOBUILD_OPTS} -o bin/${PROJECT} .

.PHONY: run
run: ## Run astrolavos from local
	go run -mod=vendor *.go -config-path ./examples/

.PHONY: docker-build
docker-build: build ## Build docker image
	docker build --build-arg GO_IMAGE=${GO_IMAGE} --build-arg DISTROLESS_IMAGE=${DISTROLESS_IMAGE} -t ${IMAGE_TAG_BASE}:${VERSION} --no-cache .

.PHONY: docker-push
docker-push: ## Push docker image
	docker push ${IMAGE_TAG_BASE}:${VERSION}

checksums:
	sha256sum bin/${PROJECT} > bin/${PROJECT}.sha256

install:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go install -v -a -mod=vendor ${GOBUILD_OPTS}
