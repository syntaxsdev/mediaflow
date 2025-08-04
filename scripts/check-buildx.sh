#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

BUILDER_NAME="multiplatform-arm64"

print_status "Checking buildx builder status..."

# Check if builder exists
if ! docker buildx ls | grep -q "$BUILDER_NAME"; then
    print_warning "Builder '$BUILDER_NAME' does not exist. Run 'make setup-buildx' to create it."
    exit 1
fi

# Check if it's the current builder
if docker buildx ls | grep -q "$BUILDER_NAME\*"; then
    print_success "Builder '$BUILDER_NAME' is the current builder"
else
    print_warning "Builder '$BUILDER_NAME' exists but is not the current builder"
    print_status "Setting it as current..."
    docker buildx use "$BUILDER_NAME"
    print_success "Builder '$BUILDER_NAME' is now the current builder"
fi

# Check if it's running
if docker buildx ls | grep -q "$BUILDER_NAME.*running"; then
    print_success "Builder '$BUILDER_NAME' is running"
else
    print_status "Builder '$BUILDER_NAME' is not running, starting it..."
    docker buildx inspect --bootstrap >/dev/null 2>&1
    if docker buildx ls | grep -q "$BUILDER_NAME.*running"; then
        print_success "Builder '$BUILDER_NAME' is now running"
    else
        print_warning "Builder '$BUILDER_NAME' may not be fully ready"
    fi
fi

# Show current status
echo
print_status "Current buildx status:"
docker buildx ls

echo
print_status "Available platforms:"
docker buildx inspect | grep -A 10 "Platforms:" | tail -n +2 | sed 's/^/  /' || echo "  Unable to get platform info" 