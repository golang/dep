#!/usr/bin/env bash
# Copyright 2017 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
#
# This script will build dep and calculate hash for each
# (DEP_BUILD_PLATFORMS, DEP_BUILD_ARCHS) pair.
# DEP_BUILD_PLATFORMS="linux" DEP_BUILD_ARCHS="amd64" ./hack/build-all.sh
# can be called to build only for linux-amd64

set -e

VERSION=$(git describe --tags --dirty)
COMMIT_HASH=$(git rev-parse --short HEAD 2>/dev/null)
DATE=$(date "+%Y-%m-%d")

GO_BUILD_CMD="go build -a -installsuffix cgo"
GO_BUILD_LDFLAGS="-s -w -X main.commitHash=$COMMIT_HASH -X main.buildDate=$DATE -X main.version=$VERSION"

if [ -z "$DEP_BUILD_PLATFORMS" ]; then
    DEP_BUILD_PLATFORMS="linux windows darwin"
fi

if [ -z "$DEP_BUILD_ARCHS" ]; then
    DEP_BUILD_ARCHS="amd64"
fi

mkdir -p release

for OS in ${DEP_BUILD_PLATFORMS[@]}; do
  for ARCH in ${DEP_BUILD_ARCHS[@]}; do
    echo "Building for $OS/$ARCH"
    GOARCH=$ARCH GOOS=$OS CGO_ENABLED=0 $GO_BUILD_CMD -ldflags "$GO_BUILD_LDFLAGS"\
     -o "release/dep-$OS-$ARCH" ./cmd/dep/
    shasum -a 256 "release/dep-$OS-$ARCH" > "release/dep-$OS-$ARCH".sha256
  done
done
