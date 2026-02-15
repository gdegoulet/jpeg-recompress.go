#!/bin/bash
set -e

VERSION="1.2.1"
BINARY="jpeg-recompress.go"
ARCHIVE="${BINARY}-v${VERSION}-linux-x86_64.tar.bz2"

echo "--- Starting Release Process for v${VERSION} ---"

# 1. Build the static binary using the existing script
echo "Building static binary v${VERSION}..."
./build-static.sh "${VERSION}"

# 2. Create the tar.bz2 archive
echo "Creating archive: ${ARCHIVE}..."
tar -cjf "${ARCHIVE}" "${BINARY}"

# 3. Generate SHA1 sum
echo "Generating SHA1 sum..."
sha1sum "${ARCHIVE}" > "${ARCHIVE}.sha1"

echo "--- Release Assets Created ---"
ls -lh "${ARCHIVE}" "${ARCHIVE}.sha1"
echo ""
echo "To finalize the release in Git, run:"
echo "git tag -a v${VERSION} -m 'Release version ${VERSION}'"
echo "git push origin v${VERSION}"
