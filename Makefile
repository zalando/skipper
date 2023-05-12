SOURCES            = $(shell find . -name '*.go' -not -path "./vendor/*" -and -not -path "./_test_plugins" -and -not -path "./_test_plugins_fail" )
PACKAGES           = $(shell go list ./...)
CURRENT_VERSION    = $(shell git describe --tags --always --dirty)
VERSION           ?= $(CURRENT_VERSION)
COMMIT_HASH        = $(shell git rev-parse --short HEAD)
LIMIT_FDS          = $(shell ulimit -n)
TEST_ETCD_VERSION ?= v2.3.8
TEST_PLUGINS       = _test_plugins/filter_noop.so \
		     _test_plugins/predicate_match_none.so \
		     _test_plugins/dataclient_noop.so \
		     _test_plugins/multitype_noop.so \
		     _test_plugins_fail/fail.so


.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

default: build

lib: $(SOURCES) ## build skipper library
	go build $(PACKAGES)

bindir:
	mkdir -p bin

skipper: $(SOURCES) bindir ## build skipper binary
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o bin/skipper ./cmd/skipper/*.go

eskip: $(SOURCES) bindir ## build eskip binary
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o bin/eskip ./cmd/eskip/*.go

webhook: $(SOURCES) bindir
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o bin/webhook ./cmd/webhook/*.go

routesrv: $(SOURCES) bindir
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o bin/routesrv ./cmd/routesrv/*.go

fixlimits:
ifeq (LIMIT_FDS, 256)
	ulimit -n 1024
endif

build: $(SOURCES) lib skipper eskip webhook routesrv ## build libe and all binaries

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

install: $(SOURCES) ## install skipper and eskip binaries into your system
	go install -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/skipper
	go install -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/eskip

check: build check-plugins ## run all tests
	# go test $(PACKAGES)
	#
	# due to vendoring and how go test ./... is not the same as go test ./a/... ./b/...
	# probably can be reverted once etcd is fully mocked away for tests
	#
	for p in $(PACKAGES); do go test $$p || break; done

shortcheck: build check-plugins fixlimits  ## run all short tests
	# go test -test.short -run ^Test $(PACKAGES)
	#
	# due to vendoring and how go test ./... is not the same as go test ./a/... ./b/...
	# probably can be reverted once etcd is fully mocked away for tests
	#
	for p in $(PACKAGES); do go test -test.short -run ^Test $$p || break -1; done

cicheck: build check-plugins ## run all short and redis tests
	# go test -test.short -run ^Test $(PACKAGES)
	#
	# due to vendoring and how go test ./... is not the same as go test ./a/... ./b/...
	# probably can be reverted once etcd is fully mocked away for tests
	#
	for p in $(PACKAGES); do go test -tags=redis -test.short -run ^Test $$p || break -1; done

check-race: build ## run all tests with race checker
	# go test -race -test.short -run ^Test $(PACKAGES)
	#
	# due to vendoring and how go test ./... is not the same as go test ./a/... ./b/...
	# probably can be reverted once etcd is fully mocked away for tests
	#
	for p in $(PACKAGES); do go test -race -test.short -run ^Test $$p || break -1; done

check-plugins: $(TEST_PLUGINS)
	go test -run LoadPlugins

_test_plugins/%.so: _test_plugins/%.go
	go build -buildmode=plugin -o $@ $<

_test_plugins_fail/%.so: _test_plugins_fail/%.go
	go build -buildmode=plugin -o $@ $<

bench: build $(TEST_PLUGINS) ## run all benchmark tests
	# go test -bench . $(PACKAGES)
	#
	# due to vendoring and how go test ./... is not the same as go test ./a/... ./b/...
	# probably can be reverted once etcd is fully mocked away for tests
	#
	for p in $(PACKAGES); do go test -bench . $$p; done

fuzz: ## run all fuzz tests
	for p in $(PACKAGES); do go test -run=NONE -fuzz=Fuzz -fuzztime 30s $$p; done

lint: build staticcheck ## run all linters

clean: ## clean temorary files and driectories
	go clean -i -cache -testcache
	rm -rf .coverprofile-all .cover
	rm -f ./_test_plugins/*.so
	rm -f ./_test_plugins_fail/*.so
	rm -rf .bin

deps: ## install dependencies to run everything
	go env
	./etcd/install.sh $(TEST_ETCD_VERSION)
	@go install honnef.co/go/tools/cmd/staticcheck@latest
	@go install github.com/securego/gosec/v2/cmd/gosec@latest
	@go install golang.org/x/vuln/cmd/govulncheck@latest

vet: $(SOURCES) ## run Go vet
	go vet $(PACKAGES)

# TODO(sszuecs) review disabling these checks, f.e.:
# -ST1000 missing package doc in many packages
# -ST1003 wrong naming convention Api vs API, Id vs ID
# -ST1012 too many error variables are not having prefix "err"
# -ST1020 too many wrong comments on exported functions to fix right away
# -ST1021 too many wrong comments on exported functions to fix right away
# -ST1022 too many wrong comments on exported functions to fix right away
staticcheck: $(SOURCES) ## run staticcheck
	staticcheck -checks "all,-ST1000,-ST1003,-ST1012,-ST1020,-ST1021" $(PACKAGES)

# TODO(sszuecs) review disabling these checks, f.e.:
# G101 find by variable name match "oauth" are not hardcoded credentials
# G104 ignoring errors are in few cases fine
# G304 reading kubernetes secret filepaths are not a file inclusions
# G307 mostly warns about defer rsp.Body.Close(), see https://github.com/securego/gosec/issues/925
# G402 See https://github.com/securego/gosec/issues/551 and https://github.com/securego/gosec/issues/528
gosec: $(SOURCES)
	gosec -quiet -exclude="G101,G104,G304,G307,G402" ./...

govulncheck: $(SOURCES) ## run govulncheck
	govulncheck ./...

fmt: $(SOURCES) ## format code
	@gofmt -w -s $(SOURCES)

check-fmt: $(SOURCES) ## check format code
	@if [ "$$(gofmt -s -d $(SOURCES))" != "" ]; then false; else true; fi

precommit: fmt build vet staticcheck check-race shortcheck ## precommit hook

.coverprofile-all: $(SOURCES) $(TEST_PLUGINS)
	# go list -f \
	# 	'{{if len .TestGoFiles}}"go test -coverprofile={{.Dir}}/.coverprofile {{.ImportPath}}"{{end}}' \
	# 	$(PACKAGES) | xargs -i sh -c {}
	#
	# due to vendoring and how go test ./... is not the same as go test ./a/... ./b/...
	# probably can be reverted once etcd is fully mocked away for tests
	#
	for p in $(PACKAGES); do \
		go list -f \
			'{{if len .TestGoFiles}}"go test -tags=redis -coverprofile={{.Dir}}/.coverprofile {{.ImportPath}}"{{end}}' \
			$$p | xargs -i sh -c {}; \
	done
	go install github.com/modocache/gover@latest
	gover . .coverprofile-all

cover: .coverprofile-all ## coverage test and show it in your browser
	go tool cover -func .coverprofile-all

show-cover: .coverprofile-all
	go tool cover -html .coverprofile-all

publish-coverage: .coverprofile-all
	curl -s https://codecov.io/bash -o codecov
	bash codecov -f .coverprofile-all
