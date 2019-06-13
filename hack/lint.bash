#!/usr/bin/env bash
# Copyright 2017 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
#
# This script will validate code with various linters
set -e

PKGS=$(go list ./... | grep -vF /vendor/)
go vet $PKGS
golint $PKGS
staticcheck $PKGS
