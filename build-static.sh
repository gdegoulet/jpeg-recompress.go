#!/bin/bash
set -e

echo "Building 100% static binary using Docker (musl)..."

VERSION=${1:-dev}

# Remove existing binary to prevent Go compiler from trying to parse it as source
rm -f jpeg-recompress.go

# Build the docker image
docker build --build-arg VERSION=$VERSION -t jpeg-recompress-builder -f Dockerfile .

# Extract the binaries from the image
echo "Extracting binaries..."
CONTAINER_ID=$(docker create jpeg-recompress-builder)
docker cp $CONTAINER_ID:/jpeg-recompress.go jpeg-recompress.go
docker cp $CONTAINER_ID:/jpegli-encode.go jpegli-encode.go
docker rm $CONTAINER_ID
chmod +x jpeg-recompress.go jpegli-encode.go

echo "Success! Binaries generated: jpeg-recompress.go, jpegli-encode.go"
file jpeg-recompress.go jpegli-encode.go
ldd jpeg-recompress.go 2>&1 || true
ldd jpegli-encode.go 2>&1 || true
