SHELL := /bin/bash # ensure bash is used

BINS := refunc loader sidecar agent

GOOS := $(shell eval $$(go env); echo $${GOOS})
ARCH := $(shell eval $$(go env); echo $${GOARCH})

LD_FLAGS := -X github.com/refunc/refunc/pkg/version.Version=$(shell source hack/scripts/version; echo $${REFUNC_VERSION}) \
-X github.com/refunc/refunc/pkg/version.AgentVersion=$(shell source hack/scripts/version; echo $${AGENT_VERSION}) \
-X github.com/refunc/refunc/pkg/version.LoaderVersion=$(shell source hack/scripts/version; echo $${LOADER_VERSION}) \
-X github.com/refunc/refunc/pkg/version.SidecarVersion=$(shell source hack/scripts/version; echo $${SIDECAR_VERSION})

clean:
	rm -rf bin/$(GOOS)

ifneq ($(GOOS),linux)
images:
	export GOOS=linux; make $@
else
images: clean $(addsuffix -image, $(BINS))
endif

bins: $(BINS)

bin/$(GOOS):
	mkdir -p $@

$(BINS): % : bin/$(GOOS) bin/$(GOOS)/%
	@source hack/scripts/common; log_info "Build: $@"

bin/$(GOOS)/%:
	@echo GOOS=$(GOOS)
	CGO_ENABLED=0 go build \
	-tags netgo -installsuffix netgo \
	-ldflags "-s -w $(LD_FLAGS)" \
	-a \
	-o $@ \
	./cmd/$*/

ifneq ($(GOOS),linux)
%-image:
	export GOOS=linux; make $@
else
%-image: % package/Dockerfile
	@rm package/$* 2>/dev/null || true && cp bin/linux/$* package/
	@ source hack/scripts/common \
	&& cd package \
	&& docker build \
	--build-arg https_proxy="$${HTTPS_RPOXY}" \
	--build-arg http_proxy="$${HTTP_RPOXY}" \
	--build-arg BIN_TARGET=$* \
	-t $(IMAGE) .
	@source hack/scripts/common; log_info "Image: $(IMAGE)"
endif

REFUNC_IMAGE=$(shell source hack/scripts/version; echo $${REFUNC_IMAGE})
bin/$(GOOS)/refunc: $(shell find pkg -type f -name '*.go') $(shell find cmd -type f -name '*.go')
refunc-image: IMAGE=$(REFUNC_IMAGE)

LOADER_IMAGE=$(shell source hack/scripts/version; echo $${LOADER_IMAGE})
bin/$(GOOS)/loader: cmd/loader/*.go pkg/runtime/lambda/loader/*.go
loader-image: IMAGE=$(LOADER_IMAGE)

SIDECAR_IMAGE=$(shell source hack/scripts/version; echo $${SIDECAR_IMAGE})
bin/$(GOOS)/loader: cmd/sidecar/*.go $(shell find pkg/sidecar -type f -name '*.go') $(shell find pkg/transport -type f -name '*.go')
sidecar-image: IMAGE=$(SIDECAR_IMAGE)

AGENT_IMAGE=$(shell source hack/scripts/version; echo $${AGENT_IMAGE})
bin/$(GOOS)/agent: cmd/agent/*.go pkg/runtime/legacy/loader/*.go
agent-image: IMAGE=$(AGENT_IMAGE)

CREDSYNCER_VERSION=$(shell source hack/scripts/version; echo $${CREDSYNCER_VERSION})
CREDSYNCER_IMAGE=$(shell source hack/scripts/version; echo $${CREDSYNCER_IMAGE})
bin/$(GOOS)/credsyncer: pkg/apis/refunc/v1beta3/*.go pkg/credsyncer/*.go cmd/credsyncer/*.go
credsyncer-image: IMAGE=$(CREDSYNCER_IMAGE)

push: images
	@source hack/scripts/common; log_info "start pushing images"
	docker push $(shell source hack/scripts/version; echo $${REFUNC_IMAGE})
	docker push $(shell source hack/scripts/version; echo $${LOADER_IMAGE})
	docker push $(shell source hack/scripts/version; echo $${SIDECAR_IMAGE})
	docker push $(shell source hack/scripts/version; echo $${AGENT_IMAGE})

build-container:
	docker build -t refunc:build -f Dockerfile.build .

dockerbuild: build-container
	@source hack/scripts/common; log_info "make bins in docker"
	@docker run --rm -it -v $(shell pwd):/go/src/github.com/refunc/refunc refunc:build make bins
