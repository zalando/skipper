.PHONY: docker-push

VERSION            ?= $(shell git rev-parse HEAD)
REGISTRY           ?= registry-write.opensource.zalan.do/pathfinder
IMAGE              ?= $(REGISTRY)/skipper:$(VERSION)
LIGHTSTEP_IMAGE    ?= $(REGISTRY)/skipper-lightstep:$(VERSION)
ALPINE_BUILD_IMAGE ?= $(REGISTRY)/skipper-alpine-build:latest
PACKAGE            ?= github.com/zalando/skipper
CGO_ENABLED        ?= 0

default: docker-build

skipper:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) go build $(PACKAGE)/cmd/skipper

eskip:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) go build $(PACKAGE)/cmd/eskip

clean:
	rm -rf skipper eskip build/

docker-build: clean skipper eskip
	docker build -t $(IMAGE) .

docker-push:
	docker push $(IMAGE)

plugins:
	GO111MODULE=on go build -buildmode=plugin -o build/tracing_lightstep.so ../opentracingplugin/tracers/lightstep/lightstep.go

skipper111:
	GO111MODULE=on go build -o skipper ../cmd/skipper/*.go

eskip111:
	GO111MODULE=on go build -o eskip ../cmd/eskip/*.go

alpine.build: docker-alpine-build
	docker run --rm \
		-v $(shell pwd)/..:/build \
		-w /build/packaging $(ALPINE_BUILD_IMAGE) \
		make skipper111 eskip111

alpine.plugins: docker-alpine-build
	docker run --rm -v $(shell pwd)/..:/build \
		-w /build/packaging $(ALPINE_BUILD_IMAGE) \
		make plugins

docker-alpine-build:
	docker build --rm -t $(ALPINE_BUILD_IMAGE) -f Dockerfile.alpine .

docker-lightstep-build: alpine.build alpine.plugins
	docker build -t $(LIGHTSTEP_IMAGE) -f Dockerfile.lightstep .

docker-lightstep-push:
	docker push $(LIGHTSTEP_IMAGE)
