#!/bin/bash
set -e

echo "Building 100% static binary using Docker (musl)..."

VERSION=${1:-dev}

# Remove existing binary to prevent Go compiler from trying to parse it as source
rm -f jpeg-recompress.go

# Build the docker image
docker build --build-arg VERSION=$VERSION -t jpeg-recompress-builder -f Dockerfile .

# Extract the binary from the image
echo "Extracting binary..."
CONTAINER_ID=$(docker create jpeg-recompress-builder)
docker cp $CONTAINER_ID:/jpeg-recompress.go jpeg-recompress.go
docker rm $CONTAINER_ID
chmod +x jpeg-recompress.go

echo "Success! Binary generated: jpeg-recompress.go"
file jpeg-recompress.go
ldd jpeg-recompress.go 2>&1 || true
