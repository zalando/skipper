.PHONY: docker-push

VERSION         ?= $(shell git rev-parse HEAD)
REGISTRY        ?= registry-write.opensource.zalan.do/pathfinder
IMAGE           ?= $(REGISTRY)/skipper:$(VERSION)
LIGHTSTEP_IMAGE ?= $(REGISTRY)/skipper-lightstep:$(VERSION)
CGO_ENABLED     ?= 0

default: docker-build

skipper:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) go build github.com/zalando/skipper/cmd/skipper

eskip:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=$(CGO_ENABLED) go build github.com/zalando/skipper/cmd/eskip

clean:
	rm -rf skipper eskip build/

docker-build: clean skipper eskip
	docker build -t $(IMAGE) .

docker-push:
	docker push $(IMAGE)

plugins:
	CUR=$(shell pwd) ; GOPATH=$(shell pwd)/build ;\
		go get -d github.com/skipper-plugins/opentracing ;\
		cd build/src/github.com/skipper-plugins/opentracing ;\
		glide install -v;\
		GOOS=linux GOARCH=amd64 CGO_ENABLED=1 make ;\
		cp build/*.so $$CUR/build/

docker-lightstep-build: skipper eskip plugins
	docker build -t $(LIGHTSTEP_IMAGE) -f Dockerfile.lightstep .

docker-lightstep-push:
	docker push $(LIGHTSTEP_IMAGE)
