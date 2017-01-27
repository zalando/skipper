SOURCES =         $(shell find . -name '*.go')
CURRENT_VERSION = $(shell git tag | sort -V | tail -n1)
VERSION ?=        $(CURRENT_VERSION)
NEXT_MAJOR =      $(shell go run packaging/version/version.go major $(currentVersion))
NEXT_MINOR =      $(shell go run packaging/version/version.go minor $(currentVersion))
NEXT_PATCH =      $(shell go run packaging/version/version.go patch $(currentVersion))
COMMIT_HASH =     $(shell git rev-parse --short HEAD)

default: build

lib: $(SOURCES)
	go build  ./...

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
	go test ./...

shortcheck: build
	go test -test.short -run ^Test ./...

bench: build
	go test -bench . ./...

clean:
	go clean -i ./...

deps:
	go get golang.org/x/sys/unix
	go get golang.org/x/crypto/ssh/terminal
	go get -t github.com/zalando/skipper/...
	./etcd/install.sh
	go get github.com/tools/godep
	godep restore ./...

vet: $(SOURCES)
	go vet ./...

fmt: $(SOURCES)
	gofmt -w $(SOURCES)

check-fmt: $(SOURCES)
	[ "$$(gofmt -d $(SOURCES))" == "" ]

precommit: build shortcheck fmt vet

check-precommit: build shortcheck check-fmt vet

ci-user:
	git config --global user.email "builds@travis-ci.com"
	git config --global user.name "Travis CI"

ci-tag: ci-user
	git tag $(VERSION)

ci-push-tags: ci-user
	git push --tags https://$(GITHUB_AUTH)@github.com/zalando/skipper

release-major:
	make VERSION=$(NEXT_MAJOR) ci-tag ci-push-tags

release-minor:
	make VERSION=$(NEXT_MINOR) ci-tag ci-push-tags

release-patch:
	make VERSION=$(NEXT_PATCH) ci-tag ci-push-tags

ci-trigger:
ifeq ($(TRAVIS_BRANCH)_$(TRAVIS_PULL_REQUEST)_$(findstring major-release,$(TRAVIS_COMMIT_MESSAGE)), master_false_major-release)
	make release-major
else ifeq ($(TRAVIS_BRANCH)_$(TRAVIS_PULL_REQUEST)_$(findstring minor-release,$(TRAVIS_COMMIT_MESSAGE)), master_false_minor-release)
	make release-minor
else ifeq ($(TRAVIS_BRANCH)_$(TRAVIS_PULL_REQUEST), master_false)
	make release-patch
else ifeq ($(TRAVIS_BRANCH), master)
	make deps check-precommit
else
	make deps shortcheck
endif
