# Change these variables as necessary.
MAIN_PACKAGE_PATH := $(shell pwd)
OS_INFO := $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH_INFO := $(shell uname -m | sed 's/x86_64/amd64/')
TMP_DIR := ./tmp
GO_EXECUTABLE := go

## tidy: format code and tidy modfile
.PHONY: tidy
tidy:
	${GO_EXECUTABLE} fmt ./...
	${GO_EXECUTABLE} mod tidy -v
	golangci-lint fmt
	golangci-lint run --fix > /dev/null 2>&1 || true
	golangci-lint run

## audit: run quality control checks
.PHONY: audit
audit:
	${GO_EXECUTABLE} mod verify
	${GO_EXECUTABLE} vet ./...
	${GO_EXECUTABLE} run honnef.co/go/tools/cmd/staticcheck@latest -checks=all,-ST1000,-U1000 ./...
	${GO_EXECUTABLE} run golang.org/x/vuln/cmd/govulncheck@latest ./...
	${GO_EXECUTABLE} test -buildvcs -vet=off ./...

## test: run all tests
.PHONY: test
test:
	CGO_ENABLED=0 ${GO_EXECUTABLE} test -v -buildvcs -vet=all ./...
	CGO_ENABLED=1 ${GO_EXECUTABLE} test -v -race -buildvcs -vet=all ./...

## test/cover: run all tests and display coverage
.PHONY: test/cover
test/cover:
	mkdir -p ${TMP_DIR}
	${GO_EXECUTABLE} test -v -buildvcs -vet=off -coverprofile=${TMP_DIR}/coverage.out ./...
	${GO_EXECUTABLE} tool cover -html=${TMP_DIR}/coverage.out

# release: build the binary and create a release
.PHONY: release
release:
	goreleaser release --clean

# new_version: create a new version
.PHONY: new_version
new_version:
	./scripts/new_version.sh $(VERSION)
