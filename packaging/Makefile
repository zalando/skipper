.PHONY: docker-push

VERSION            ?= $(shell git rev-parse HEAD)
REGISTRY           ?= registry-write.opensource.zalan.do/pathfinder
IMAGE              ?= $(REGISTRY)/skipper:$(VERSION)
LIGHTSTEP_IMAGE    ?= $(REGISTRY)/skipper-lightstep:$(VERSION)
OPENTRACING_IMAGE    ?= $(REGISTRY)/skipper-opentracing:$(VERSION)
ALPINE_BUILD_IMAGE ?= $(REGISTRY)/skipper-alpine-build:latest
PACKAGE            ?= github.com/zalando/skipper
CGO_ENABLED        ?= 0
GOOS               ?= linux
GOARCH             ?= amd64
GOARM              ?=
GO111              ?= on

default: docker-build

skipper:
	GO111MODULE=$(GO111) GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOARM) CGO_ENABLED=$(CGO_ENABLED) go build -o skipper ../cmd/skipper/*.go

eskip:
	GO111MODULE=$(GO111) GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOARM) CGO_ENABLED=$(CGO_ENABLED) go build -o eskip ../cmd/eskip/*.go

clean:
	rm -rf skipper eskip build/

docker-build: clean skipper eskip
	docker build -t $(IMAGE) .

docker-push:
	docker push $(IMAGE)

build.linux: $(SOURCES)
	GO111MODULE=$(GO111) GOOS=linux GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) go build -o build/linux/skipper ../cmd/skipper/*.go
	GO111MODULE=$(GO111) GOOS=linux GOARCH=arm64 CGO_ENABLED=$(CGO_ENABLED) go build -o build/linux_arm8/skipper ../cmd/skipper/*.go
	GO111MODULE=$(GO111) GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=$(CGO_ENABLED) go build -o build/linux_arm7/skipper ../cmd/skipper/*.go

build.osx:
	GO111MODULE=$(GO111) GOOS=darwin GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) go build -o build/osx/skipper -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ../cmd/skipper

build.windows:
	GO111MODULE=$(GO111) GOOS=windows GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) go build -o build/windows/skipper.exe -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" ../cmd/skipper

build.package: build.linux build.osx build.windows
	tar --transform 's,^\.,skipper-$(VERSION)-linux-amd64,' -C build/linux -czvf skipper-$(VERSION)-linux-amd64.tar.gz .
	tar --transform 's,^\.,skipper-$(VERSION)-linux-arm8,' -C build/linux_arm8 -czvf skipper-$(VERSION)-linux-arm8.tar.gz .
	tar --transform 's,^\.,skipper-$(VERSION)-linux-arm7,' -C build/linux_arm7 -czvf skipper-$(VERSION)-linux-arm7.tar.gz .
	tar --transform 's,^\.,skipper-$(VERSION)-osx-amd64,' -C build/osx -czvf skipper-$(VERSION)-osx-amd64.tar.gz .
	tar --transform 's,^\.,skipper-$(VERSION)-windows-amd64,' -C build/windows -czvf skipper-$(VERSION)-windows-amd64.tar.gz .
	for f in *.tar.gz; do sha256sum $$f >> sha256sum.txt; done

