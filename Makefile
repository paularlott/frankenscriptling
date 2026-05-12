IMAGE_NAME ?= frankenscriptling
IMAGE_TAG ?= 1.12.2

.PHONY: help build-apple build-docker build-docker-push

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build-apple: ## Build for local arch using Apple Containers
	container build -t $(IMAGE_NAME):$(IMAGE_TAG) .

build-docker: ## Build multi-arch (amd64+arm64) using Docker buildx
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-t $(IMAGE_NAME):$(IMAGE_TAG) \
		.

build-docker-push: ## Build and push multi-arch image to registry
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-t $(IMAGE_NAME):$(IMAGE_TAG) \
		--push \
		.
