SOURCES            = $(shell find . -name '*.go' -not -path "./vendor/*" -and -not -path "./_test_plugins" -and -not -path "./_test_plugins_fail" )
PACKAGES           = $(shell go list ./...)
CURRENT_VERSION    = $(shell git describe --tags --always --dirty)
VERSION           ?= $(CURRENT_VERSION)
NEXT_MAJOR         = $(shell go run packaging/version/version.go major $(CURRENT_VERSION))
NEXT_MINOR         = $(shell go run packaging/version/version.go minor $(CURRENT_VERSION))
NEXT_PATCH         = $(shell go run packaging/version/version.go patch $(CURRENT_VERSION))
COMMIT_HASH        = $(shell git rev-parse --short HEAD)
LIMIT_FDS          = $(shell ulimit -n)
TEST_ETCD_VERSION ?= v2.3.8
TEST_PLUGINS       = _test_plugins/filter_noop.so \
		     _test_plugins/predicate_match_none.so \
		     _test_plugins/dataclient_noop.so \
		     _test_plugins/multitype_noop.so \
		     _test_plugins_fail/fail.so
GO111             ?= on

default: build

lib: $(SOURCES)
	GO111MODULE=$(GO111) go build $(PACKAGES)

bindir:
	mkdir -p bin

skipper: $(SOURCES) bindir
	GO111MODULE=$(GO111) go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o bin/skipper ./cmd/skipper/*.go

eskip: $(SOURCES) bindir
	GO111MODULE=$(GO111) go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o bin/eskip ./cmd/eskip/*.go

fixlimits:
ifeq (LIMIT_FDS, 256)
	ulimit -n 1024
endif

build: $(SOURCES) lib skipper eskip

build.linux.armv8:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 GO111MODULE=on go build -o bin/skipper -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/skipper

build.linux.armv7:
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 GO111MODULE=on go build -o bin/skipper -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/skipper

build.linux:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GO111MODULE=on go build -o bin/skipper -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/skipper

build.osx:
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 GO111MODULE=on go build -o bin/skipper -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/skipper

build.windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 GO111MODULE=on go build -o bin/skipper -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/skipper

install: $(SOURCES)
	GO111MODULE=on go install -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/skipper
	GO111MODULE=on go install -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/eskip

check: build check-plugins
	# go test $(PACKAGES)
	#
	# due to vendoring and how go test ./... is not the same as go test ./a/... ./b/...
	# probably can be reverted once etcd is fully mocked away for tests
	#
	for p in $(PACKAGES); do GO111MODULE=on go test $$p || break; done

shortcheck: build check-plugins fixlimits
	# go test -test.short -run ^Test $(PACKAGES)
	#
	# due to vendoring and how go test ./... is not the same as go test ./a/... ./b/...
	# probably can be reverted once etcd is fully mocked away for tests
	#
	for p in $(PACKAGES); do GO111MODULE=on go test -test.short -run ^Test $$p || break -1; done

cicheck: build check-plugins
	# go test -test.short -run ^Test $(PACKAGES)
	#
	# due to vendoring and how go test ./... is not the same as go test ./a/... ./b/...
	# probably can be reverted once etcd is fully mocked away for tests
	#
	for p in $(PACKAGES); do GO111MODULE=on go test -tags=redis -test.short -run ^Test $$p || break -1; done

check-race: build
	# go test -race -test.short -run ^Test $(PACKAGES)
	#
	# due to vendoring and how go test ./... is not the same as go test ./a/... ./b/...
	# probably can be reverted once etcd is fully mocked away for tests
	#
	for p in $(PACKAGES); do GO111MODULE=on go test -race -test.short -run ^Test $$p || break -1; done

check-plugins: $(TEST_PLUGINS)
	GO111MODULE=on go test -run LoadPlugins

_test_plugins/%.so: _test_plugins/%.go
	GO111MODULE=on go build -buildmode=plugin -o $@ $<

_test_plugins_fail/%.so: _test_plugins_fail/%.go
	GO111MODULE=on go build -buildmode=plugin -o $@ $<

bench: build $(TEST_PLUGINS)
	# go test -bench . $(PACKAGES)
	#
	# due to vendoring and how go test ./... is not the same as go test ./a/... ./b/...
	# probably can be reverted once etcd is fully mocked away for tests
	#
	for p in $(PACKAGES); do GO111MODULE=on go test -bench . $$p; done

lint: build staticcheck

clean:
	go clean -i -cache -testcache ./...
	rm -rf .coverprofile-all .cover
	rm -f ./_test_plugins/*.so
	rm -f ./_test_plugins_fail/*.so

deps:
	go env
	./etcd/install.sh $(TEST_ETCD_VERSION)
	@curl -o /tmp/staticcheck -LO https://github.com/dominikh/go-tools/releases/download/2019.1/staticcheck_linux_amd64
	@sha256sum /tmp/staticcheck | grep -q a13563b3fe136674a87e174bbedbd1af49e5bd89ffa605a11150ae06ab9fd999
	@mkdir -p $(GOPATH)/bin
	@mv /tmp/staticcheck $(GOPATH)/bin/
	@chmod +x $(GOPATH)/bin/staticcheck
	@curl -o /tmp/gosec.tgz -LO https://github.com/securego/gosec/releases/download/2.0.0/gosec_2.0.0_linux_amd64.tar.gz
	@sha256sum /tmp/gosec.tgz | grep -q 490c2a0434b2b9cbb2f4c5031eafe228023f1ac41b36dddd757bff9e1de76a2b
	@tar -C /tmp -xzf /tmp/gosec.tgz
	@mkdir -p $(GOPATH)/bin
	@mv /tmp/gosec $(GOPATH)/bin/
	@chmod +x $(GOPATH)/bin/gosec
	@which gosec || cp -a $(GOPATH)/bin/gosec $(GOBIN)/gosec

vet: $(SOURCES)
	GO111MODULE=on go vet $(PACKAGES)

# TODO(sszuecs) review disabling these checks, f.e.:
# -ST1000 missing package doc in many packages
# -ST1003 wrong naming convention Api vs API, Id vs ID
# -ST1012 too many error variables are not having prefix "err"
staticcheck: $(SOURCES)
	GO111MODULE=on staticcheck -checks "all,-ST1000,-ST1003,-ST1012" $(PACKAGES)

# TODO(sszuecs) review disabling these checks, f.e.:
# G101 find by variable name match "oauth" are not hardcoded credentials
# G104 ignoring errors are in few cases fine
# G304 reading kubernetes secret filepaths are not a file inclusions
gosec: $(SOURCES)
	GO111MODULE=on gosec -quiet -exclude="G101,G104,G304" ./...

fmt: $(SOURCES)
	@gofmt -w -s $(SOURCES)

check-fmt: $(SOURCES)
	@if [ "$$(gofmt -s -d $(SOURCES))" != "" ]; then false; else true; fi

precommit: fmt build vet staticcheck check-race shortcheck

check-precommit: check-fmt build vet staticcheck check-race cicheck gosec

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
			'{{if len .TestGoFiles}}"GO111MODULE=on go test -coverprofile={{.Dir}}/.coverprofile {{.ImportPath}}"{{end}}' \
			$$p | xargs -i sh -c {}; \
	done
	go get github.com/modocache/gover
	gover . .coverprofile-all

cover: .coverprofile-all
	go tool cover -func .coverprofile-all

show-cover: .coverprofile-all
	go tool cover -html .coverprofile-all

publish-coverage: .coverprofile-all
	curl -s https://codecov.io/bash -o codecov
	bash codecov -f .coverprofile-all

release-major:
	echo $(NEXT_MAJOR)

release-minor:
	echo $(NEXT_MINOR)

release-patch:
	echo $(NEXT_PATCH)

ci-trigger:
ifeq ($(TRAVIS_BRANCH)_$(TRAVIS_PULL_REQUEST)_$(findstring major-release,$(TRAVIS_COMMIT_MESSAGE)), master_false_major-release)
	make deps publish-coverage
else ifeq ($(TRAVIS_BRANCH)_$(TRAVIS_PULL_REQUEST)_$(findstring minor-release,$(TRAVIS_COMMIT_MESSAGE)), master_false_minor-release)
	make deps publish-coverage
else ifeq ($(TRAVIS_BRANCH)_$(TRAVIS_PULL_REQUEST), master_false)
	make deps publish-coverage
else ifeq ($(TRAVIS_BRANCH), master)
	make deps check-precommit
else
	make deps check-race cicheck check-plugins
endif
