.PHONY: docker-push

VERSION            ?= $(shell git rev-parse HEAD)
REGISTRY           ?= dist.spotahome.net:5000/spotahome
IMAGE              ?= $(REGISTRY)/skipper-custom:$(VERSION)
LIGHTSTEP_IMAGE    ?= $(REGISTRY)/skipper-custom-lightstep:$(VERSION)
OPENTRACING_IMAGE    ?= $(REGISTRY)/skipper-custom-opentracing:$(VERSION)
ALPINE_BUILD_IMAGE ?= $(REGISTRY)/skipper-custom-alpine-build:latest
PACKAGE            ?= github.com/zalando/skipper
CGO_ENABLED        ?= 0
GO111              ?= on

default: docker-build

skipper:
	GO111MODULE=$(GO111) GOOS=linux GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) go build -o skipper ../cmd/skipper/*.go

eskip:
	GO111MODULE=$(GO111) GOOS=linux GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) go build -o eskip ../cmd/eskip/*.go

clean:
	rm -rf skipper eskip build/

docker-build: clean skipper eskip
	docker build -t $(IMAGE) .

docker-push:
	docker push $(IMAGE)
