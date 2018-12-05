SHELL := /bin/bash # ensure bash is used

BINS := refunc loader sidecar agent

OS   := $(shell eval $$(go env); echo $${GOOS})
ARCH := $(shell eval $$(go env); echo $${GOARCH})

LD_FLAGS := -X github.com/refunc/refunc/pkg/version.Version=$(shell source hack/scripts/version; echo $${REFUNC_VERSION}) \
-X github.com/refunc/refunc/pkg/version.AgentVersion=$(shell source hack/scripts/version; echo $${AGENT_VERSION}) \
-X github.com/refunc/refunc/pkg/version.LoaderVersion=$(shell source hack/scripts/version; echo $${LOADER_VERSION}) \
-X github.com/refunc/refunc/pkg/version.SidecarVersion=$(shell source hack/scripts/version; echo $${SIDECAR_VERSION})

images: $(addsuffix -image, $(BINS))

bins: $(BINS)

bin/$(OS):
	mkdir -p $@

$(BINS): % : bin/$(OS) bin/$(OS)/%
	@source hack/scripts/common; log_info "Building $@ Done"

bin/$(OS)/%:
	CGO_ENABLED=0 go build \
	-tags netgo -installsuffix netgo \
	-ldflags "-s -w $(LD_FLAGS)" \
	-a \
	-o $@ \
	./cmd/$*/

%-image: % package/Dockerfile
	@rm package/$* 2>/dev/null || true && cp bin/linux/$* package/
	@ source hack/scripts/common \
	&& cd package \
	&& docker build \
	--build-arg https_proxy="$${HTTPS_RPOXY}" \
	--build-arg http_proxy="$${HTTP_RPOXY}" \
	--build-arg BIN_TARGET=$* \
	-t $(IMAGE) .

AGENT_IMAGE=$(shell source hack/scripts/version; echo $${AGENT_IMAGE})
REFUNC_IMAGE=$(shell source hack/scripts/version; echo $${REFUNC_IMAGE})
bin/$(OS)/refunc: LD_FLAGS := $(LD_FLAGS) -X github.com/refunc/refunc/pkg/runtime/refunc/runtime.InitContainerImage=$(AGENT_IMAGE)
bin/$(OS)/refunc: $(shell find pkg -type f -name '*.go') $(shell find cmd -type f -name '*.go')
refunc-image: IMAGE=$(REFUNC_IMAGE)
refunc-image: agent-image

LOADER_IMAGE=$(shell source hack/scripts/version; echo $${LOADER_IMAGE})
bin/$(OS)/loader: cmd/loader/*.go pkg/runtime/lambda/loader/*.go
loader-image: IMAGE=$(LOADER_IMAGE)

SIDECAR_IMAGE=$(shell source hack/scripts/version; echo $${SIDECAR_IMAGE})
bin/$(OS)/loader: cmd/sidecar/*.go $(shell find pkg/sidecar -type f -name '*.go')
loader-image: IMAGE=$(LOADER_IMAGE)

AGENT_IMAGE=$(shell source hack/scripts/version; echo $${AGENT_IMAGE})
bin/$(OS)/agent: cmd/agent/*.go pkg/runtime/refunc/loader/*.go
agent-image: IMAGE=$(AGENT_IMAGE)

CREDSYNCER_VERSION=$(shell source hack/scripts/version; echo $${CREDSYNCER_VERSION})
CREDSYNCER_IMAGE=$(shell source hack/scripts/version; echo $${CREDSYNCER_IMAGE})
bin/$(OS)/credsyncer: pkg/apis/refunc/v1beta3/*.go pkg/credsyncer/*.go cmd/credsyncer/*.go
credsyncer-image: IMAGE=$(CREDSYNCER_IMAGE)

versions: images
	@echo 'controller: ' >values.images.yaml; \
	echo '  image: refunc:${REFUNC_VERSION}' >>values.images.yaml; \
	echo 'credsyncer: ' >>values.images.yaml; \
	echo '  image: credsyncer:${CREDSYNCER_VERSION}' >>values.images.yaml; \
	echo 'triggers: ' >>values.images.yaml; \
	echo '  eventTrigger: ' >>values.images.yaml; \
	echo '    image: refunc:${REFUNC_VERSION}' >>values.images.yaml; \
	echo '  timeTrigger: ' >>values.images.yaml; \
	echo '    image: refunc:${REFUNC_VERSION}' >>values.images.yaml; \
	echo '  httpTrigger: ' >>values.images.yaml; \
	echo '    image: refunc:${REFUNC_VERSION}' >>values.images.yaml;
