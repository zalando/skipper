SOURCES            = $(shell find . -name '*.go' -not -path "./vendor/*")
PACKAGES           = $(shell glide novendor || echo -n "./...")
CURRENT_VERSION    = $(shell git tag | sort -V | tail -n1)
VERSION           ?= $(CURRENT_VERSION)
NEXT_MAJOR         = $(shell go run packaging/version/version.go major $(CURRENT_VERSION))
NEXT_MINOR         = $(shell go run packaging/version/version.go minor $(CURRENT_VERSION))
NEXT_PATCH         = $(shell go run packaging/version/version.go patch $(CURRENT_VERSION))
COMMIT_HASH        = $(shell git rev-parse --short HEAD)
TEST_ETCD_VERSION ?= v2.3.8

default: build

lib: $(SOURCES)
	go build $(PACKAGES)

bindir:
	mkdir -p bin

skipper: $(SOURCES) bindir
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o bin/skipper ./cmd/skipper

eskip: $(SOURCES) bindir
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o bin/eskip ./cmd/eskip

build: $(SOURCES) lib skipper eskip

install: $(SOURCES)
	go install -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/skipper
	go install -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ./cmd/eskip

check: build
	# go test $(PACKAGES)
	#
	# due to vendoring and how go test ./... is not the same as go test ./a/... ./b/...
	# probably can be reverted once etcd is fully mocked away for tests
	#
	for p in $(PACKAGES); do go test $$p || break; done

shortcheck: build
	# go test -test.short -run ^Test $(PACKAGES)
	#
	# due to vendoring and how go test ./... is not the same as go test ./a/... ./b/...
	# probably can be reverted once etcd is fully mocked away for tests
	#
	for p in $(PACKAGES); do go test -test.short -run ^Test $$p || break -1; done

bench: build
	# go test -bench . $(PACKAGES)
	#
	# due to vendoring and how go test ./... is not the same as go test ./a/... ./b/...
	# probably can be reverted once etcd is fully mocked away for tests
	#
	for p in $(PACKAGES); do go test -bench . $$p; done

lint: build
	gometalinter --enable-all --deadline=60s ./... | tee linter.log

clean:
	go clean -i ./...

deps:
	go get -t github.com/zalando/skipper/...
	./etcd/install.sh $(TEST_ETCD_VERSION)
	go get github.com/Masterminds/glide
	glide install --strip-vendor
	# get opentracing to the default GOPATH, so we can build plugins outside
	# the main skipper repo
	# * will be removed from vendor/ after the deps checks (workaround for glide list)
	go get -t github.com/opentracing/opentracing-go
	# fix vendored deps:
	rm -rf vendor/github.com/sirupsen/logrus/examples # breaks go install ./...

vet: $(SOURCES)
	go vet $(PACKAGES)

fmt: $(SOURCES)
	@gofmt -w $(SOURCES)

check-fmt: $(SOURCES)
	@if [ "$$(gofmt -d $(SOURCES))" != "" ]; then false; else true; fi

check-imports:
	@glide list && true || \
	(echo "run make deps and check if any new dependencies were vendored with glide get" && \
	false)
	# workaround until glide list supports --ignore $PACKAGE:
	rm -rf vendor/github.com/opentracing/opentracing-go

precommit: check-imports fmt build shortcheck vet

check-precommit: check-imports check-fmt build shortcheck vet

.coverprofile-all: $(SOURCES)
	# go list -f \
	# 	'{{if len .TestGoFiles}}"go test -coverprofile={{.Dir}}/.coverprofile {{.ImportPath}}"{{end}}' \
	# 	$(PACKAGES) | xargs -i sh -c {}
	#
	# due to vendoring and how go test ./... is not the same as go test ./a/... ./b/...
	# probably can be reverted once etcd is fully mocked away for tests
	#
	for p in $(PACKAGES); do \
		go list -f \
			'{{if len .TestGoFiles}}"go test -coverprofile={{.Dir}}/.coverprofile {{.ImportPath}}"{{end}}' \
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

tag:
	git tag $(VERSION)

push-tags:
	git push --tags https://$(GITHUB_AUTH)@github.com/zalando/skipper

release-major:
	make VERSION=$(NEXT_MAJOR) tag push-tags

release-minor:
	make VERSION=$(NEXT_MINOR) tag push-tags

release-patch:
	make VERSION=$(NEXT_PATCH) tag push-tags

ci-user:
	git config --global user.email "builds@travis-ci.com"
	git config --global user.name "Travis CI"

ci-release-major: ci-user deps release-major
ci-release-minor: ci-user deps release-minor
ci-release-patch: ci-user deps release-patch

ci-trigger:
ifeq ($(TRAVIS_BRANCH)_$(TRAVIS_PULL_REQUEST)_$(findstring major-release,$(TRAVIS_COMMIT_MESSAGE)), master_false_major-release)
	make deps publish-coverage ci-release-major
else ifeq ($(TRAVIS_BRANCH)_$(TRAVIS_PULL_REQUEST)_$(findstring minor-release,$(TRAVIS_COMMIT_MESSAGE)), master_false_minor-release)
	make deps publish-coverage ci-release-minor
else ifeq ($(TRAVIS_BRANCH)_$(TRAVIS_PULL_REQUEST), master_false)
	make deps publish-coverage ci-release-patch
else ifeq ($(TRAVIS_BRANCH), master)
	make deps check-precommit
else
	make deps shortcheck
endif
