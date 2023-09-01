VERSION            ?= $(shell git rev-parse HEAD)
REGISTRY           ?= registry-write.opensource.zalan.do/teapot
BINARIES           ?= skipper webhook eskip routesrv
IMAGE              ?= $(REGISTRY)/skipper:$(VERSION)
ARM64_IMAGE        ?= $(REGISTRY)/skipper-arm64:$(VERSION)
ARM_IMAGE          ?= $(REGISTRY)/skipper-armv7:$(VERSION)
MULTIARCH_IMAGE    ?= container-registry-test.zalando.net/teapot/skipper:$(VERSION)
CGO_ENABLED        ?= 0
COMMIT_HASH        = $(shell git rev-parse --short HEAD)

.PHONY: help
help: ## display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-.]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: clean
clean: ## clean binaries
	rm -rf sha256sum.txt *.tar.gz build/

.PHONY: docker.build.local
docker.build.local: clean $(addprefix build/,$(BINARIES)) ## build local architecture image
	docker build -t $(IMAGE) --build-arg TARGETPLATFORM= .

.PHONY: docker.build.amd64
docker.build.amd64: clean build.linux.amd64 docker.build.enable ## build linux/amd64 image using a trusted amd64 alpine base image
	docker buildx build -t $(IMAGE) --platform linux/amd64 --load . \
		--build-arg BASE_IMAGE=registry.opensource.zalan.do/library/alpine-3:latest

.PHONY: docker.build.arm64
docker.build.arm64: clean build.linux.arm64 docker.build.enable ## build linux/arm64 image
	docker buildx build -t $(ARM64_IMAGE) --platform linux/arm64 --load .

.PHONY: docker.build.armv7
docker.build.armv7: clean build.linux.armv7 docker.build.enable ## build linux/arm/v7 image
	docker buildx build -t $(ARM_IMAGE) --platform linux/arm/v7 --load .

.PHONY: docker.push.local
docker.push.local: ## push local image
	docker push $(IMAGE)

.PHONY: docker.push.amd64
docker.push.amd64: ## push linux/amd64 image
	docker push $(IMAGE)

.PHONY: docker.push.arm64
docker.push.arm64: ## push linux/arm64 image
	docker push $(ARM64_IMAGE)

.PHONY: docker.push.armv7
docker.push.armv7: ## push linux/arm/v7 image
	docker push $(ARM_IMAGE)

# Currently it is not possible to build and push multiarch image separately,
# see https://github.com/docker/buildx/issues/59
.PHONY: docker.push.multiarch
docker.push.multiarch: clean build.linux docker.build.enable ## build and push multi-arch image using a trusted multi-arch alpine base image
	docker buildx build --rm -t $(MULTIARCH_IMAGE) --platform linux/amd64,linux/arm64 --push \
		--build-arg BASE_IMAGE=container-registry.zalando.net/library/alpine-3:latest .

# https://docs.docker.com/build/working-with-build/
# ~/.docker/config.json add: "experimental": "enabled",
docker.build.enable:
	[ $$(which jq) ] || echo "please install jq"
	[ -d $$HOME/.docker ] || mkdir $$HOME/.docker
	[ -f $$HOME/.docker/config.json ] || touch $$HOME/.docker/config.json
	if [ -s $$HOME/.docker/config.json ]; then jq -r '. += {experimental: "enabled"}' $$HOME/.docker/config.json > $$HOME/.docker/config.json.new; mv $$HOME/.docker/config.json.new $$HOME/.docker/config.json; else echo '{"experimental": "enabled"}' >$$HOME/.docker/config.json; fi

.PHONY: build.linux build.linux.amd64 build.linux.arm64 build.linux.armv7
build.linux: build.linux.amd64 build.linux.arm64 build.linux.armv7
build.linux.amd64: $(addprefix build/linux/amd64/,$(BINARIES))
build.linux.arm64: $(addprefix build/linux/arm64/,$(BINARIES))
build.linux.armv7: $(addprefix build/linux/arm/v7/,$(BINARIES))

.PHONY: build.darwin build.darwin.amd64 build.darwin.arm64
build.darwin: build.darwin.amd64 build.darwin.arm64
build.darwin.amd64: $(addprefix build/darwin/amd64/,$(BINARIES))
build.darwin.arm64: $(addprefix build/darwin/arm64/,$(BINARIES))

.PHONY: build.windows
build.windows: $(addprefix build/windows/amd64/,$(BINARIES))

# local build
build/%:
	$(GOARM) \
	CGO_ENABLED=$(CGO_ENABLED) \
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o $@ ../cmd/$(notdir $@)

build/linux/amd64/%:
	GOOS=linux \
	GOARCH=amd64 \
	CGO_ENABLED=$(CGO_ENABLED) \
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o $@ ../cmd/$(notdir $@)

build/linux/arm64/%:
	GOOS=linux \
	GOARCH=arm64 \
	CGO_ENABLED=$(CGO_ENABLED) \
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o $@ ../cmd/$(notdir $@)

build/linux/arm/v7/%:
	GOOS=linux \
	GOARCH=arm \
	GOARM=7 \
	CGO_ENABLED=$(CGO_ENABLED) \
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o $@ ../cmd/$(notdir $@)

build/darwin/amd64/%:
	GOOS=darwin \
	GOARCH=amd64 \
	CGO_ENABLED=$(CGO_ENABLED) \
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o $@ ../cmd/$(notdir $@)

build/darwin/arm64/%:
	GOOS=darwin \
	GOARCH=arm64 \
	CGO_ENABLED=$(CGO_ENABLED) \
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o $@ ../cmd/$(notdir $@)

build/windows/amd64/%:
	GOOS=windows \
	GOARCH=amd64 \
	CGO_ENABLED=$(CGO_ENABLED) \
	go build -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o $@ ../cmd/$(notdir $@)

.PHONY: build.package
build.package: build.linux build.darwin build.windows ## package binaries
	tar --transform 's,^\.,skipper-$(VERSION)-linux-amd64,' -C build/linux/amd64 -czvf skipper-$(VERSION)-linux-amd64.tar.gz .
	tar --transform 's,^\.,skipper-$(VERSION)-linux-arm64,' -C build/linux/arm64 -czvf skipper-$(VERSION)-linux-arm64.tar.gz .
	tar --transform 's,^\.,skipper-$(VERSION)-linux-armv7,' -C build/linux/arm/v7 -czvf skipper-$(VERSION)-linux-armv7.tar.gz .
	tar --transform 's,^\.,skipper-$(VERSION)-darwin-amd64,' -C build/darwin/amd64 -czvf skipper-$(VERSION)-darwin-amd64.tar.gz .
	tar --transform 's,^\.,skipper-$(VERSION)-darwin-arm64,' -C build/darwin/arm64 -czvf skipper-$(VERSION)-darwin-arm64.tar.gz .
	tar --transform 's,^\.,skipper-$(VERSION)-windows-amd64,' -C build/windows/amd64 -czvf skipper-$(VERSION)-windows-amd64.tar.gz .
	for f in *.tar.gz; do sha256sum $$f >> sha256sum.txt; done

.PHONY: test
test: ## test build targets using https://github.com/multiarch/qemu-user-static
	IMAGE=skipper-packaging-test:local make docker.build.local
	IMAGE=skipper-packaging-test:local make test-run

	IMAGE=skipper-packaging-test:amd64 make docker.build.amd64
	IMAGE=skipper-packaging-test:amd64 TARGETPLATFORM=linux/amd64 make test-run

	ARM64_IMAGE=skipper-packaging-test:arm64 make docker.build.arm64
	IMAGE=skipper-packaging-test:arm64 TARGETPLATFORM=linux/arm64 make test-run

	ARM_IMAGE=skipper-packaging-test:armv7 make docker.build.armv7
	IMAGE=skipper-packaging-test:armv7 TARGETPLATFORM=linux/arm/v7 make test-run

.PHONY: test-run
test-run:
	docker run --platform=$(TARGETPLATFORM) --rm -it $(IMAGE) uname -m
	docker run --platform=$(TARGETPLATFORM) --rm -it $(IMAGE) skipper -version
