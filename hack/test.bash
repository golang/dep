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

if ! ./dep status -out .dep.status.file.output; then exit 1; fi
if ! ./dep status > .dep.status.stdout.output; then
   rm -f .dep.status.file.output
   exit 1
fi
if ! diff .dep.status.file.output .dep.status.stdout.output; then
  diffResult=1
else
  diffResult=0
fi
rm -f .dep.status.file.output .dep.status.stdout.output
if [ "$diffResult" -eq "1" ]; then
  exit 1
fi
