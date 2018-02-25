#!/usr/bin/env bash
# Copyright 2017 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
#
# This script will build dep and calculate hash for each
# (DEP_BUILD_PLATFORMS, DEP_BUILD_ARCHS) pair.
# DEP_BUILD_PLATFORMS="linux" DEP_BUILD_ARCHS="amd64" ./hack/build-all.bash
# can be called to build only for linux-amd64

set -e

IMPORT_DURING_SOLVE=${IMPORT_DURING_SOLVE:-false}

go test -race \
    -ldflags '-X github.com/golang/dep/cmd/dep.flagImportDuringSolve=${IMPORT_DURING_SOLVE}' \
    ./...
