SOURCES =         $(shell find . -name '*.go')
CURRENT_VERSION = $(shell git tag | sort -V | tail -n1)
VERSION =         $(CURRENT_VERSION)
NEXT_MAJOR =      $(shell go run packaging/version/version.go major $(currentVersion))
NEXT_MINOR =      $(shell go run packaging/version/version.go minor $(currentVersion))
NEXT_PATCH =      $(shell go run packaging/version/version.go patch $(currentVersion))
COMMIT_HASH =     $(shell git rev-parse --short HEAD)

default: build

build: $(SOURCES)
	go build ./...

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
	if [ "$$(gofmt -d $(SOURCES))" != "" ]; then false; else true; fi

precommit: build shortcheck fmt vet

check-precommit: build shortcheck check-fmt vet
