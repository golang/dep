#!/usr/bin/env bash
# Copyright 2017 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
#
# This script will validate that `go fmt` has been ran
# and is passing for certain directories in the project.
#
# Here we use `go list` to help determine which packages
# we need to check for `go fmt`
#
# EXIT 0 - The check is successful
# EXIT 1 - The check has failed

PKGS=$(go list ./... | grep -v /vendor/)
REPO_TLD="github.com/golang/dep"
IGNORE_PKGS=". ./gps"

for PKG in $PKGS; do
    RELATIVE_PATH="${PKG/$REPO_TLD/.}"
    i=0
    for IGNORE_PKG in $IGNORE_PKGS; do
        if [ "${IGNORE_PKG}" == $RELATIVE_PATH ]; then
            i=1
        fi
    done;
    if [ $i -eq 1 ]; then
        continue
    fi

    echo "Processing gofmt for: ${PKG}"
    gofmt -s -l $RELATIVE_PATH
    if [ $? -ne 0 ]; then
        echo "GO FMT FAILURE: ${PKG}"
        exit 1
    fi
done;
exit 0
