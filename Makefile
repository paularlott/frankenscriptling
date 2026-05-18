IMAGE_NAME ?= frankenscriptling
FRANKENPHP_VERSION ?= 1.12.3
PHP_VERSION ?= 8.5.6
SCRIPTLING_VERSION ?= v0.8.1
REGISTRY ?= paularlott
IMAGE_TAG ?= 1.12.3-php$(PHP_VERSION)

BUILD_ARGS = \
	--build-arg FRANKENPHP_VERSION=$(FRANKENPHP_VERSION) \
	--build-arg PHP_VERSION=$(PHP_VERSION) \
	--build-arg SCRIPTLING_VERSION=$(SCRIPTLING_VERSION)

.PHONY: help build-apple build-docker build-docker-push test

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build-apple: ## Build for local arch using Apple Containers
	container build $(BUILD_ARGS) -t $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) .

build-docker: ## Build multi-arch (amd64+arm64) using Docker buildx
	docker buildx build \
		$(BUILD_ARGS) \
		--platform linux/amd64,linux/arm64 \
		-t $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) \
		.

build-docker-push: ## Build and push multi-arch image to registry
	docker buildx build \
		$(BUILD_ARGS) \
		--platform linux/amd64,linux/arm64 \
		-t $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) \
		--push \
		.

test: ## Run PHP tests inside the container
	container run --rm \
		-v $(CURDIR)/tests:/app/tests:ro \
		$(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) \
		frankenphp php-cli /app/tests/test_all.php
