THIS_DIR                := $(realpath $(dir $(abspath $(firstword $(MAKEFILE_LIST)))))
IMAGE_REPO              := brilliantcreator
IMAGE_NAME              := ollama-authentication-proxy
IMAGE_GOLANG_VERSION    := 1.25.1
IMAGE_OLLAMA_VERSION    := 0.12.3
IMAGE_TAG_POSTFIX       := 20251008
IMAGE_TAG               := $(IMAGE_OLLAMA_VERSION)_$(IMAGE_TAG_POSTFIX)
IMAGE_TAGGED            := $(IMAGE_REPO)/$(IMAGE_NAME):$(IMAGE_TAG)
IMAGE_LATEST            := $(IMAGE_REPO)/$(IMAGE_NAME):latest
DOCKER_BUILDX_DRIVER    := docker-container
DOCKER_BUILDX_BUILDER   := container-builder
DOCKER_BUILDX_PLATFORMS := linux/arm64,linux/amd64
CONTAINER_NAME          := ollama-authentication-proxy
LOCAL_PORT              := 18434

ollama-authentication-proxy: *.go go.mod
	go build -gcflags="-N -l" .

debug-ollama-authentication-proxy:: ollama-authentication-proxy
	dlv --listen=:2345 --headless=true --api-version=2 --accept-multiclient exec ollama-authentication-proxy

# To build a multi platform/architecture docker image, you need to setup a builder context:
# See https://docs.docker.com/build/building/multi-platform/
create_pipeline_builder_context::
	@docker buildx ls | grep -E "^$(DOCKER_BUILDX_BUILDER)(\*?)\s+$(DOCKER_BUILDX_DRIVER)" >/dev/null \
		|| docker buildx create --name $(DOCKER_BUILDX_BUILDER) --driver $(DOCKER_BUILDX_DRIVER) --bootstrap
	@docker buildx use $(DOCKER_BUILDX_BUILDER)

# Find out what platform/architecture you can build.
show_pipeline_image_builder_platforms:: create_pipeline_builder_context
	@echo Docker builder \"$(DOCKER_BUILDX_BUILDER)\" supports the following platforms/architectures:
	@docker buildx inspect --builder $(DOCKER_BUILDX_BUILDER) --bootstrap | grep -i "platforms:" | cut -d: -f2 | xargs

# Build a multi platform/architecture docker image.
# The built image will be imported from builder-context into current docker-context ( `--output=type=docker` ).
LOAD_IMAGE := false # whether to load the built image into current docker-context ( this only works if you build for a single platform/architecture ! )
PUSH_IMAGE := false # whether to push the built image to docker registry
NO_CACHE := false # whether to disable cache during docker build
build-image:: create_pipeline_builder_context
	docker buildx build \
		--builder $(DOCKER_BUILDX_BUILDER) \
		--platform=$(DOCKER_BUILDX_PLATFORMS) \
		$(shell $(NO_CACHE) && echo --no-cache) \
		$(shell $(LOAD_IMAGE) && echo --output=type=docker) \
		--output=type=image \
		--debug \
		--progress plain \
		$(shell $(PUSH_IMAGE) && echo --push) \
		--build-arg GOLANG_VERSION=$(IMAGE_GOLANG_VERSION) \
		--build-arg OLLAMA_VERSION=$(IMAGE_OLLAMA_VERSION) \
		--label org.opencontainers.image.title="ollama-authentication-proxy" \
		--label org.opencontainers.image.description="Ollama ($(IMAGE_OLLAMA_VERSION)) behind authentication proxy" \
		--label org.opencontainers.image.documentation="https://github.com/manuel-koch/ollama-authentication-proxy" \
		-t $(IMAGE_TAGGED) \
		-t $(IMAGE_LATEST) \
		-f $(THIS_DIR)/Dockerfile .
	@echo
	@echo "Built docker image: $(IMAGE_TAGGED)"
	@$(PUSH_IMAGE) && echo "Pushed docker image: $(IMAGE_TAGGED)"

# build docker image for arm64 and load it into local docker-context
build-image-arm64:: LOAD_IMAGE=true
build-image-arm64:: DOCKER_BUILDX_PLATFORMS=linux/arm64
build-image-arm64:: build-image
	docker inspect --format='{{json .Config.Labels}}' $(IMAGE_TAGGED)

run-image::
	docker run --rm \
		--name $(CONTAINER_NAME) \
		-v $(THIS_DIR)/ollama:/root/.ollama \
		-p $(LOCAL_PORT):80 \
		-e "AUTHORIZATION_APIKEY=my-private-api-key" \
		-e "PRELOAD_MODEL=qwen3:0.6b" \
		-e "OLLAMA_HOST=127.0.0.1:11434" \
		$(IMAGE_TAGGED)

run-image-interactive::
	docker run -it --rm \
		--name $(CONTAINER_NAME) \
		-v $(THIS_DIR)/ollama:/root/.ollama \
		-p $(LOCAL_PORT):80 \
		-e "AUTHORIZATION_APIKEY=my-private-api-key" \
		-e "PRELOAD_MODEL=qwen3:0.6b" \
		-e "OLLAMA_HOST=127.0.0.1:11434" \
		--entrypoint "sh" \
		$(IMAGE_TAGGED) \
		-i
