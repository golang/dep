#!/usr/bin/env bash
# Copyright 2017 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
#
# This script will build licenseok and run it on all
# source files to check licence
set -e

go build ./hack/licenseok
find . -path ./vendor -prune -o -regex ".+\.pb\.go$" -prune -o -type f -regex ".*\.\(go\|proto\)$"\
 -printf '%P\n' | xargs ./licenseok
