.PHONY: docker-push

VERSION            ?= $(shell git rev-parse HEAD)
REGISTRY           ?= registry-write.opensource.zalan.do/teapot
BINARIES           ?= skipper webhook eskip routesrv
IMAGE              ?= $(REGISTRY)/skipper:$(VERSION)
ARM64_IMAGE        ?= $(REGISTRY)/skipper-arm64:$(VERSION)
ARM_IMAGE          ?= $(REGISTRY)/skipper-armv7:$(VERSION)
PACKAGE            ?= github.com/zalando/skipper
CGO_ENABLED        ?= 0
GOOS               ?= linux
GOARCH             ?= amd64
GOARM              ?=
BUILD_FLAGS        ?= -v -trimpath
COMMIT_HASH        = $(shell git rev-parse --short HEAD)

default: docker-build

skipper:
	GOOS=$(GOOS) \
	GOARCH=$(GOARCH) \
	$(GOARM) \
	CGO_ENABLED=$(CGO_ENABLED) \
	go build $(BUILD_FLAGS) -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o skipper ../cmd/skipper/*.go

eskip:
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOARM) CGO_ENABLED=$(CGO_ENABLED) go build $(BUILD_FLAGS) -o eskip ../cmd/eskip/*.go

webhook:
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOARM) CGO_ENABLED=$(CGO_ENABLED) go build $(BUILD_FLAGS) -o webhook ../cmd/webhook/*.go

routesrv:
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOARM) CGO_ENABLED=$(CGO_ENABLED) go build $(BUILD_FLAGS) -o routesrv ../cmd/routesrv/*.go

clean:
	rm -rf $(BINARIES) build/

docker.build: clean $(BINARIES)
	docker build -t $(IMAGE) --build-arg BUILD_FOLDER=. --build-arg TARGETPLATFORM= .

docker-build: docker.build.amd64 docker.build.arm64 docker.build.armv7
docker.build.amd64: clean build.linux.amd64 docker.build.enable
	docker buildx build -t $(IMAGE) --platform linux/amd64 -f Dockerfile --load .
docker.build.arm64: clean build.linux.arm64 docker.build.enable
	docker buildx build -t $(ARM64_IMAGE) --platform linux/arm64 -f Dockerfile.arm64 --load .
docker.build.armv7: clean build.linux.armv7 docker.build.enable
	docker buildx build -t $(ARM_IMAGE) --platform linux/arm/v7 -f Dockerfile.armv7 --load .

docker-push: docker.push.amd64 docker.push.arm64 docker.push.armv7
docker.push.amd64:
	docker push $(IMAGE)
docker.push.arm64: docker.build.arm64
	docker push $(ARM64_IMAGE)
docker.push.armv7: docker.build.armv7
	docker push $(ARM_IMAGE)

# build multi-arch container image using a trusted multi-arch base image
docker.push.multiarch: clean build.linux docker.build.enable
	docker buildx build --rm -t $(MULTIARCH_IMAGE) --platform linux/amd64,linux/arm64 --push \
	  --build-arg BASE_IMAGE=container-registry.zalando.net/library/alpine-3:latest .

# https://docs.docker.com/build/working-with-build/
# ~/.docker/config.json add: "experimental": "enabled",
docker.build.enable:
	[ $$(which jq) ] || echo "please install jq"
	[ -d $$HOME/.docker ] || mkdir $$HOME/.docker
	[ -f $$HOME/.docker/config.json ] || touch $$HOME/.docker/config.json
	if [ -s $$HOME/.docker/config.json ]; then jq -r '. += {experimental: "enabled"}' $$HOME/.docker/config.json > $$HOME/.docker/config.json.new; mv $$HOME/.docker/config.json.new $$HOME/.docker/config.json; else echo '{"experimental": "enabled"}' >$$HOME/.docker/config.json; fi

build.linux: build.linux.amd64 build.linux.arm64 build.linux.armv7
build.linux.amd64: $(addprefix build/linux/amd64/,$(BINARIES))
build.linux.arm64: $(addprefix build/linux/arm64/,$(BINARIES))
build.linux.armv7: $(addprefix build/linux/arm/v7/,$(BINARIES))

build.darwin: build.darwin.amd64 build.darwin.arm64
build.darwin.amd64: $(addprefix build/darwin/amd64/,$(BINARIES))
build.darwin.arm64: $(addprefix build/darwin/arm64/,$(BINARIES))

build.windows: $(addprefix build/windows/amd64/,$(BINARIES))

build/linux/amd64/%:
	GOOS=linux \
	GOARCH=amd64 \
	CGO_ENABLED=$(CGO_ENABLED) \
	go build $(BUILD_FLAGS) -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o build/linux/amd64/$(notdir $@) ../cmd/$(notdir $@)/*.go

build/linux/arm64/%:
	GOOS=linux \
	GOARCH=arm64 \
	CGO_ENABLED=$(CGO_ENABLED) \
	go build $(BUILD_FLAGS) -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o build/linux/arm64/$(notdir $@) ../cmd/$(notdir $@)/*.go

build/linux/arm/v7/%:
	GOOS=linux \
	GOARCH=arm \
	GOARM=7 \
	CGO_ENABLED=$(CGO_ENABLED) \
	go build $(BUILD_FLAGS) -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o build/linux/arm/v7/$(notdir $@) ../cmd/$(notdir $@)/*.go

build/darwin/amd64/%:
	GOOS=darwin \
	GOARCH=amd64 \
	CGO_ENABLED=$(CGO_ENABLED) \
	go build $(BUILD_FLAGS) -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o build/darwin/amd64/$(notdir $@) ../cmd/$(notdir $@)/*.go

build/darwin/arm64/%:
	GOOS=darwin \
	GOARCH=arm64 \
	CGO_ENABLED=$(CGO_ENABLED) \
	go build $(BUILD_FLAGS) -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o build/darwin/arm64/$(notdir $@) ../cmd/$(notdir $@)/*.go

build/windows/amd64/%:
	GOOS=windows \
	GOARCH=amd64 \
	CGO_ENABLED=$(CGO_ENABLED) \
	go build $(BUILD_FLAGS) -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT_HASH)" -o build/windows/amd64/$(notdir $@).exe ../cmd/$(notdir $@)/*.go

build.package: build.linux build.darwin build.windows
# Pack linux binaries as tar
	tar --transform 's,^\.,skipper-$(VERSION)-linux-amd64,' -C build/linux/amd64 -czvf skipper-$(VERSION)-linux-amd64.tar.gz .
	tar --transform 's,^\.,skipper-$(VERSION)-linux-arm64,' -C build/linux/arm64 -czvf skipper-$(VERSION)-linux-arm64.tar.gz .
	tar --transform 's,^\.,skipper-$(VERSION)-linux-armv7,' -C build/linux/arm/v7 -czvf skipper-$(VERSION)-linux-armv7.tar.gz .

# Pack darwin binaries as tar
	tar --transform 's,^\.,skipper-$(VERSION)-darwin-amd64,' -C build/darwin/amd64 -czvf skipper-$(VERSION)-darwin-amd64.tar.gz .
	tar --transform 's,^\.,skipper-$(VERSION)-darwin-arm64,' -C build/darwin/arm64 -czvf skipper-$(VERSION)-darwin-arm64.tar.gz .

# Pack windows binaries as zip - use a temp directory to achieve same transform result as other archives
	cp -R build/windows/amd64/ skipper-$(VERSION)-windows-amd64
	zip -r skipper-$(VERSION)-windows-amd64.zip skipper-$(VERSION)-windows-amd64
	rm -rf skipper-$(VERSION)-windows-amd64

# Generate checksums for all archives and save to file
	sha256sum *.tar.gz *.zip > sha256sum.txt
