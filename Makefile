SOURCES             = $(shell find . -name '*.go' -and -not -path "./_test_plugins" -and -not -path "./_test_plugins_fail" )
PACKAGES            = $(shell go list ./...)
CURRENT_VERSION     = $(shell git describe --tags --always --dirty)
VERSION            ?= $(CURRENT_VERSION)
COMMIT_HASH         = $(shell git rev-parse --short HEAD)
LIMIT_FDS           = $(shell ulimit -n)
TEST_ETCD_VERSION  ?= v3.5.11
TEST_ETCD_CHECKSUM ?= 4fb304f384dd4d6e491e405fed8375a09ea1c6c2596b93f97cb31844202e620df160f87f18611e84f17675e7b7245e40d1aa23571ecdb507cb094ba04d378171
TEST_PLUGINS       = _test_plugins/filter_noop.so \
		     _test_plugins/predicate_match_none.so \
		     _test_plugins/dataclient_noop.so \
		     _test_plugins/multitype_noop.so \
		     _test_plugins_fail/fail.so

.PHONY: default
default: build

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: lib
lib: $(SOURCES) ## build skipper library
	go build ./...

.PHONY: skipper
skipper: $(SOURCES) ## build skipper binary
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o bin/skipper ./cmd/skipper

.PHONY: eskip
eskip: $(SOURCES) ## build eskip binary
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o bin/eskip ./cmd/eskip

.PHONY: webhook
webhook: $(SOURCES) ## build webhook binary
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o bin/webhook ./cmd/webhook

.PHONY: routesrv
routesrv: $(SOURCES) ## build routesrv binary
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o bin/routesrv ./cmd/routesrv

.PHONY: fixlimits
fixlimits:
ifeq (LIMIT_FDS, 256)
	ulimit -n 1024
endif

.PHONY: build
build: $(SOURCES) lib skipper eskip webhook routesrv ## build library and all binaries

build.linux.static: ## build static linux binary for amd64
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/skipper -ldflags "-extldflags=-static -X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/skipper

build.linux.arm64: ## build linux binary for arm64
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bin/skipper -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/skipper

build.linux.armv7: ## build linux binary for arm7
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -o bin/skipper -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/skipper

build.linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bin/skipper -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/skipper

build.darwin.arm64: ## build osx binary for arm64
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o bin/skipper -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/skipper

build.darwin: ## build osx binary for amd64
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o bin/skipper -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/skipper

build.windows: ## build windows binary for amd64
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o bin/skipper -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/skipper

.PHONY: install
install: $(SOURCES) ## install skipper and eskip binaries into your system
	go install -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/skipper
	go install -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/eskip

.PHONY: check
check: build check-plugins ## run all tests
	go test ./...

.PHONY: shortcheck
shortcheck: build check-plugins fixlimits  ## run all short tests
	go test -test.short ./...

.PHONY: check-race
check-race: build ## run all short tests with race checker
	go test -race -test.short ./...

.PHONY: check-plugins
check-plugins: $(TEST_PLUGINS)
	go test -run LoadPlugins

_test_plugins/%.so: _test_plugins/%.go
	go build -buildmode=plugin -o $@ $<

_test_plugins_fail/%.so: _test_plugins_fail/%.go
	go build -buildmode=plugin -o $@ $<

.PHONY: fuzz
fuzz: ## run all fuzz tests
	$(MAKE) -C fuzz $(MAKECMDGOALS)

.PHONY: lint
lint: build staticcheck ## run all linters

.PHONY: clean
clean: ## clean temporary files and directories
	go clean -i ./...
	rm -rf bin
	rm -rf .bin
	rm -f _test_plugins/*.so
	rm -f _test_plugins_fail/*.so
	rm -f .coverprofile-all coverage.out

.PHONY: deps
deps: ## install dependencies to run everything
	go env
	./etcd/install.sh $(TEST_ETCD_VERSION) $(TEST_ETCD_CHECKSUM)
	@go install honnef.co/go/tools/cmd/staticcheck@latest
	@go install github.com/securego/gosec/v2/cmd/gosec@latest
	@go install golang.org/x/vuln/cmd/govulncheck@latest
	@go install github.com/google/osv-scanner/cmd/osv-scanner@v1
	@go install github.com/google/capslock/cmd/capslock@latest

.PHONY: vet
vet: $(SOURCES) ## run Go vet
	go vet ./...

.PHONY: staticcheck
# TODO(sszuecs) review disabling these checks, f.e.:
# -ST1003 wrong naming convention Api vs API, Id vs ID
# -ST1020 too many wrong comments on exported functions to fix right away
staticcheck: $(SOURCES) ## run staticcheck
	staticcheck -checks "all,-ST1003,-ST1020" ./...

.PHONY: gosec
# TODO(sszuecs) review disabling these checks, f.e.:
# G101 find by variable name match "oauth" are not hardcoded credentials
# G104 ignoring errors are in few cases fine
# G304 reading kubernetes secret filepaths are not a file inclusions
# G307 mostly warns about defer rsp.Body.Close(), see https://github.com/securego/gosec/issues/925
# G402 See https://github.com/securego/gosec/issues/551 and https://github.com/securego/gosec/issues/528
gosec: $(SOURCES)
	gosec -quiet -exclude="G101,G104,G304,G307,G402" ./...

.PHONY: govulncheck
govulncheck: $(SOURCES) ## run govulncheck
	govulncheck ./...

.PHONY: capslock
capslock: ## run capslock
	capslock -output=v -packages=./...

.PHONY: osv-scanner
osv-scanner: $(SOURCES) ## run osv-scanner see https://osv.dev/
	osv-scanner -r ./

.PHONY: fmt
fmt: $(SOURCES) ## format code
	@gofmt -w -s $(SOURCES)

.PHONY: check-fmt
check-fmt: $(SOURCES) ## check format code
	@if [ "$$(gofmt -s -d $(SOURCES))" != "" ]; then false; else true; fi

.PHONY: precommit
precommit: fmt build vet staticcheck check-race shortcheck ## precommit hook

coverprofile: $(SOURCES) $(TEST_PLUGINS)
	go test -test.short -covermode atomic -coverprofile=coverage.out ./...

.coverprofile-all: $(SOURCES) $(TEST_PLUGINS)
	go test -test.short -coverprofile=.coverprofile-all ./...

.PHONY: cover
cover: .coverprofile-all ## coverage test and show it in your browser
	go tool cover -func .coverprofile-all

.PHONY: show-cover
show-cover: .coverprofile-all
	go tool cover -html .coverprofile-all
