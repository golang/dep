#!/usr/bin/env bash
# Copyright 2017 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.
#
# This script checks if we changed anything with regard to dependency management
# for our repo and makes sure that it was done in a valid way.

set -e -o pipefail

if [ -z "$VALIDATE_UPSTREAM" ]; then
	VALIDATE_REPO='https://github.com/golang/dep.git'
	VALIDATE_BRANCH='master'

	VALIDATE_HEAD="$(git rev-parse --verify HEAD)"

	git fetch -q "$VALIDATE_REPO" "refs/heads/$VALIDATE_BRANCH"
	VALIDATE_UPSTREAM="$(git rev-parse --verify FETCH_HEAD)"

	VALIDATE_COMMIT_DIFF="$VALIDATE_UPSTREAM...$VALIDATE_HEAD"

	validate_diff() {
		if [ "$VALIDATE_UPSTREAM" != "$VALIDATE_HEAD" ]; then
			git diff "$VALIDATE_COMMIT_DIFF" "$@"
		fi
	}
fi

IFS=$'\n'
files=( $(validate_diff --diff-filter=ACMR --name-only -- 'Gopkg.toml' 'Gopkg.lock' 'vendor/' || true) )
unset IFS

if [ ${#files[@]} -gt 0 ]; then
	go build ./cmd/dep
	./dep ensure -vendor-only
	./dep prune
	# Let see if the working directory is clean
	diffs="$(git status --porcelain -- vendor Gopkg.toml Gopkg.lock 2>/dev/null)"
	if [ "$diffs" ]; then
		{
			echo 'The contents of vendor differ after "dep ensure && dep prune":'
			echo
			echo "$diffs"
			echo
			echo 'Make sure these commands have been run before committing.'
			echo
		} >&2
		false
	else
		echo 'Congratulations! All vendoring changes are done the right way.'
	fi
else
    echo 'No vendor changes in diff.'
fi
