.PHONY: docker-push

VERSION  ?= $(shell git rev-parse HEAD)
REGISTRY ?= registry-write.opensource.zalan.do/pathfinder
IMAGE    ?= $(REGISTRY)/skipper:$(VERSION)

SOURCES = $(shell find .. -name '*.go')

default: docker-build

skipper: $(SOURCES)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build github.com/zalando/skipper/cmd/skipper

eskip: $(SOURCES)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build github.com/zalando/skipper/cmd/eskip

docker-build: skipper eskip
	docker build -t $(IMAGE) .

docker-push:
	docker push $(IMAGE)
