.PHONY: docker-push

VERSION  ?= $(shell git rev-parse HEAD)
REGISTRY ?= registry-write.opensource.zalan.do/pathfinder
IMAGE    ?= $(REGISTRY)/skipper:$(VERSION)

default: docker-build

skipper:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build github.com/zalando/skipper/cmd/skipper

eskip:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build github.com/zalando/skipper/cmd/eskip

skoap:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build github.com/zalando/skipper/cmd/skoap

clean:
	rm -f skipper eskip skoap

docker-build: clean skipper eskip skoap
	docker build -t $(IMAGE) .

docker-push:
	docker push $(IMAGE)
