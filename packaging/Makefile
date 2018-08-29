.PHONY: docker-push

VERSION         ?= $(shell git rev-parse HEAD)
REGISTRY        ?= registry-write.opensource.zalan.do/pathfinder
IMAGE           ?= $(REGISTRY)/skipper:$(VERSION)
LIGHTSTEP_IMAGE ?= $(REGISTRY)/skipper-lightstep:$(VERSION)

default: docker-build

skipper:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build github.com/zalando/skipper/cmd/skipper

eskip:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build github.com/zalando/skipper/cmd/eskip

clean:
	rm -rf skipper eskip build/ *.so

docker-build: clean skipper eskip
	docker build -t $(IMAGE) .

docker-push:
	docker push $(IMAGE)

plugins:
	CUR=$(shell pwd) ; GOPATH=$(shell pwd)/build ;\
		go get -d github.com/skipper-plugins/opentracing ;\
		cd build/src/github.com/skipper-plugins/opentracing ;\
		GOOS=linux GOARCH=amd64 CGO_ENABLED=1 make ;\

docker-lightstep-build: clean docker-build plugins
	docker build -t $(LIGHTSTEP_IMAGE) --build-arg VERSION=$(VERSION) -f Dockerfile.lightstep .

docker-lightstep-push:
	docker push $(LIGHTSTEP_IMAGE)
