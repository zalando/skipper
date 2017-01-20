SOURCES = $(shell find . -name '*.go')

default: build

build: $(SOURCES)
	go build ./...

install: $(SOURCES)
	go install ./...

check: build
	go test ./...

shortcheck: build
	go test -test.short -run ^Test ./...

bench: build
	go test -bench . ./...

vet: $(SOURCES)
	go vet ./...

fmt:
	gofmt -w $(SOURCES)

checkfmt:
	if [[ $$(gofmt -d $(SOURCES)) ]]; then false; else true; fi

precommit: build shortcheck fmt vet

check-precommit: build shortcheck checkfmt vet
