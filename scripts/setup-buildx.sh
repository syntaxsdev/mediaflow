#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Builder configuration
BUILDER_NAME="multiplatform-arm64"
BUILDER_DRIVER="docker-container"

print_status "Setting up Docker buildx multiplatform builder..."

# Check if Docker is running
if ! docker info >/dev/null 2>&1; then
    print_error "Docker is not running or not accessible"
    exit 1
fi

# Check if buildx is available
if ! docker buildx version >/dev/null 2>&1; then
    print_error "Docker buildx is not available"
    exit 1
fi

# Check if the builder already exists
if docker buildx ls | grep -q "$BUILDER_NAME"; then
    print_status "Builder '$BUILDER_NAME' already exists"
    
    # Check if it's the current builder
    if docker buildx ls | grep -q "$BUILDER_NAME\*"; then
        print_success "Builder '$BUILDER_NAME' is already the current builder"
    else
        print_status "Setting '$BUILDER_NAME' as the current builder..."
        docker buildx use "$BUILDER_NAME"
        print_success "Builder '$BUILDER_NAME' is now the current builder"
    fi
else
    print_status "Creating new builder '$BUILDER_NAME'..."
    docker buildx create --name "$BUILDER_NAME" --driver "$BUILDER_DRIVER" --use
    print_success "Builder '$BUILDER_NAME' created and set as current"
fi

# Bootstrap the builder to ensure it's ready
print_status "Bootstrapping builder..."
docker buildx inspect --bootstrap >/dev/null 2>&1

# Verify the builder is working and start it if needed
if docker buildx ls | grep -q "$BUILDER_NAME.*running"; then
    print_success "Builder '$BUILDER_NAME' is running and ready"
else
    print_status "Starting builder '$BUILDER_NAME'..."
    docker buildx inspect --bootstrap >/dev/null 2>&1
    if docker buildx ls | grep -q "$BUILDER_NAME.*running"; then
        print_success "Builder '$BUILDER_NAME' is now running"
    else
        print_warning "Builder '$BUILDER_NAME' may not be fully ready - this is normal for container-based builders"
    fi
fi

# Show available platforms
print_status "Available platforms:"
docker buildx inspect | grep -A 10 "Platforms:" | tail -n +2 | sed 's/^/  /'

print_success "Multiplatform builder setup complete!"
print_status "You can now use 'make build-image' to build for multiple platforms" 