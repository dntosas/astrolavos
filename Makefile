COMMIT = $(shell git log --pretty=format:'%h' -n 1)
VERSION= $(shell git describe --abbrev=0 --exact-match || echo development)
GOBUILD_OPTS = -ldflags="-s -w -X main.Version=${VERSION} -X main.CommitHash=${COMMIT}"
GO_IMAGE_LINT = "golangci/golangci-lint:v1.42.1"

.PHONY: release
release:
	./release.py ${stack}

.PHONY: release-major
release-major:
	./release.py ${stack} --major

.PHONY: release-minor
release-minor:
	./release.py ${stack} --minor

.PHONY: release-patch
release-patch:
	./release.py ${stack} --patch

.PHONY: release-rc
release-rc:
	./release.py --rc ${stack}

.PHONY: cleanup-rc
cleanup-rc:
	./release.py --cleanup-rc

lint:
	docker run --rm -v $(PWD):/app -w /app ${GO_IMAGE_LINT} golangci-lint run --enable revive,bodyclose,gofmt,exportloopref --exclude-use-default=false --modules-download-mode=vendor --build-tags integration

modsync:
	go mod tidy && go mod vendor

# Use following target to run directly astrolavos and test functionalities
# You can run it like `make run ARGS="version -h"`
run:
	go run -mod=vendor *.go ${ARGS}

install:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go install -v -a -mod=vendor ${GOBUILD_OPTS}

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -mod=vendor ${GOBUILD_OPTS} -o ./bin/astrolavos

darwinbuild:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -a -mod=vendor ${GOBUILD_OPTS} -o ./bin/astrolavos-darwin

build-all:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -a -mod=vendor ${GOBUILD_OPTS} -o ./bin/astrolavos-darwin && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -mod=vendor ${GOBUILD_OPTS} -o ./bin/astrolavos

fmt:
	go fmt ./...

test:
	go test -mod=vendor `go list ./... ` -race

help:
	@echo "Please use 'make <target>' where <target> is one of the following:"
	@echo "  run             to run the app without building."
	@echo "  build-all       to build the app for both Linux and MacOSX."
	@echo "  build           to build the app for Linux."
	@echo "  darwinbuild     to build the app for MacOSX."
	@echo "  lint            to perform linting."
	@echo "  fmt             to perform formatting."
	@echo "  modsync         to perform mod tidy and vendor."
	@echo "  release-major   to release a new major verson."
	@echo "  release-minor   to release a new minor verson."
	@echo "  release-patch   to release a new patch verson."
	@echo "  test            to run application tests."

.PHONY: help lint run build darwinbuild fmt
