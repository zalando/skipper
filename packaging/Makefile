.PHONY: docker-push

SKIPPER_VERSION  ?= $(shell git rev-parse HEAD)
SKIPPER_REGISTRY ?= registry-write.opensource.zalan.do/pathfinder
SKIPPER_IMAGE    ?= $(SKIPPER_REGISTRY)/skipper:$(SKIPPER_VERSION)

SOURCES = $(shell find .. -name '*.go')

default: docker-build

skipper: $(SOURCES)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build github.com/zalando/skipper/cmd/skipper

eskip: $(SOURCES)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build github.com/zalando/skipper/cmd/eskip

docker-build: skipper eskip
	docker build -t $(SKIPPER_IMAGE) .

docker-push:
	docker push $(SKIPPER_IMAGE)
