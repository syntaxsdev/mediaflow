IMAGE_NAME=mediaflow
IMAGE_TAG=latest
IMAGE_REPO=docker.io/syntaxsdev
IMAGE_FULL_NAME=$(IMAGE_REPO)/$(IMAGE_NAME):$(IMAGE_TAG)
# ARCH := $(shell uname -m)

run: build
	@echo "Starting server 🚀"
	@set -a && . ./.env && ./mediaflow

run-air:
	@echo "Starting server with air 🚀"
	@set -a && . ./.env && air


build:
	@echo "Building server 🔨"
	@go build -o mediaflow main.go
	@echo "Server built successfully 🎉"

setup-buildx:
	@echo "Setting up multiplatform builder 🔧"
	@./scripts/setup-buildx.sh

check-buildx:
	@echo "Checking buildx builder status 🔍"
	@./scripts/check-buildx.sh

stop-buildx:
	@echo "Stopping buildx builder 🛑"
	@./scripts/stop-buildx.sh

build-image: setup-buildx
	@echo "Building image for AMD64 and 386 🔨"
	@docker buildx build --platform linux/amd64,linux/386 -t $(IMAGE_FULL_NAME) .
	@echo "Image built successfully 🎉"
	@echo "Note: Multi-platform images are not loaded locally. Use --push to push to registry."

build-image-arm64: setup-buildx
	@echo "Building image for ARM64 🔨"
	@DOCKER_BUILDKIT=1 docker buildx build --platform linux/arm64 --builder default --load -t $(IMAGE_FULL_NAME) -f Dockerfile .
	@echo "ARM64 image built successfully 🎉"

build-image-all: setup-buildx
	@echo "Building image for all supported platforms 🔨"
	@docker buildx build --platform linux/amd64,linux/386 -t $(IMAGE_FULL_NAME) .
	@echo "Multi-platform image built successfully 🎉"
	@echo "Note: Multi-platform images are not loaded locally. Use --push to push to registry."

push-image: setup-buildx
	@echo "Building and pushing multi-platform image 🔨"
	@docker buildx build --platform linux/amd64,linux/386 -t $(IMAGE_FULL_NAME) --push .
	@echo "Multi-platform image built and pushed successfully 🎉"

run-image:
	@echo "Running image 🚀"
	@set -a && . ./.env && docker run -p 8080:8080 --replace -n mediaflow-server --rm $(IMAGE_FULL_NAME)

clean:
	@echo "Cleaning up 🧹"
	@rm -f mediaflow
