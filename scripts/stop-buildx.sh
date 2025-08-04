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

print_status "Stopping buildx builder..."

# Check if builder exists
if ! docker buildx ls | grep -q "$BUILDER_NAME"; then
    print_warning "Builder '$BUILDER_NAME' does not exist"
    exit 0
fi

# Check if it's running
if docker buildx ls | grep -q "$BUILDER_NAME.*running"; then
    print_status "Stopping builder '$BUILDER_NAME'..."
    docker buildx stop "$BUILDER_NAME" 2>/dev/null || true
    print_success "Builder '$BUILDER_NAME' stopped"
else
    print_status "Builder '$BUILDER_NAME' is already stopped"
fi

# Show current status
echo
print_status "Current buildx status:"
docker buildx ls

print_success "Builder stopped successfully!"
print_status "Run 'make setup-buildx' to start it again when needed" 