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
